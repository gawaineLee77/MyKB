package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const (
	ModernProtocol = "2026-07-28"
	LegacyProtocol = "2025-11-25"
)

type PrincipalResolver interface {
	CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error)
}

type ToolCaller interface {
	Call(context.Context, string, json.RawMessage, authorization.Principal, http.Header, string) (any, error)
}

type Limiter interface {
	Allow(string, time.Time) bool
}

type Handler struct {
	principals PrincipalResolver
	tools      ToolCaller
	limiter    Limiter
	version    string
}

func NewHandler(principals PrincipalResolver, tools ToolCaller, limiter Limiter, productVersion string) (*Handler, error) {
	if principals == nil || tools == nil || limiter == nil || strings.TrimSpace(productVersion) == "" {
		return nil, fmt.Errorf("MCP transport dependencies are required")
	}
	return &Handler{principals: principals, tools: tools, limiter: limiter, version: productVersion}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodDelete {
		w.Header().Set("Allow", "POST")
		writeHTTPError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeHTTPError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !validOrigin(r) {
		writeHTTPError(w, http.StatusForbidden, "origin_denied")
		return
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" && strings.TrimSpace(r.Header.Get("X-API-Key")) == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mindcreek-mcp"`)
		writeHTTPError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	principalHeaders := r.Header.Clone()
	principalHeaders.Del("X-Tenant-ID")
	principal, err := h.principals.CurrentPrincipal(r.Context(), principalHeaders)
	if err != nil || principal.User == nil || principal.User.ID == "" || principal.Tenant == nil || principal.Tenant.ID == 0 {
		writeHTTPError(w, http.StatusUnauthorized, "authentication_invalid")
		return
	}
	if requested := strings.TrimSpace(r.Header.Get("X-Tenant-ID")); requested != "" {
		id, parseErr := strconv.ParseUint(requested, 10, 64)
		if parseErr != nil || id != principal.Tenant.ID {
			writeHTTPError(w, http.StatusForbidden, "workspace_denied")
			return
		}
	}
	limitKey := fmt.Sprintf("%d:%s", principal.Tenant.ID, principal.User.ID)
	if !h.limiter.Allow(limitKey, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		writeHTTPError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeHTTPError(w, http.StatusUnsupportedMediaType, "content_type_invalid")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeHTTPError(w, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	var request rpcRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || ensureJSONEOF(decoder) != nil || request.JSONRPC != "2.0" || request.Method == "" {
		writeRPC(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error"}})
		return
	}
	modern := r.Header.Get("MCP-Protocol-Version") == ModernProtocol
	if err := validateTransportRequest(r, request, modern); err != nil {
		writeRPC(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: request.ID, Error: &rpcError{Code: -32020, Message: "Protocol header or metadata mismatch"}})
		return
	}
	if len(request.ID) == 0 || string(request.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "server/discover":
		if !modern {
			response.Error = &rpcError{Code: -32601, Message: "Method not found"}
			break
		}
		response.Result = h.discoveryResult()
	case "initialize":
		if modern {
			response.Error = &rpcError{Code: -32601, Message: "Method not found"}
			break
		}
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(request.Params, &params) != nil || !supportedLegacy(params.ProtocolVersion) {
			response.Error = &rpcError{Code: -32602, Message: "Unsupported protocol version"}
			break
		}
		response.Result = map[string]any{
			"protocolVersion": params.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]bool{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "mindcreek", "version": h.version},
			"instructions":    "Read-only access to knowledge authorized for the authenticated MindCreek user.",
		}
	case "ping":
		response.Result = completeResult(map[string]any{})
	case "tools/list":
		response.Result = h.toolList(modern)
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.Name == "" {
			response.Error = &rpcError{Code: -32602, Message: "Invalid tool arguments"}
			break
		}
		value, callErr := h.tools.Call(r.Context(), params.Name, params.Arguments,
			authorization.Principal{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, r.Header, correlationID(r))
		if callErr != nil {
			log.Printf("mindcreek MCP tool=%q correlation=%q failed: %v", params.Name, correlationID(r), callErr)
			response.Error = toolRPCError(callErr)
			break
		}
		encoded, _ := json.Marshal(value)
		response.Result = map[string]any{
			"resultType": "complete", "content": []map[string]string{{"type": "text", "text": string(encoded)}},
			"structuredContent": value, "isError": false,
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "Method not found"}
	}
	if modern && response.Result != nil {
		if result, ok := response.Result.(map[string]any); ok {
			result["_meta"] = map[string]any{"io.modelcontextprotocol/serverInfo": map[string]string{"name": "mindcreek", "version": h.version}}
		}
	}
	writeRPC(w, http.StatusOK, response)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func validateTransportRequest(r *http.Request, request rpcRequest, modern bool) error {
	accept := strings.ToLower(r.Header.Get("Accept"))
	if (modern || accept != "") && (!strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream")) {
		return ErrInvalid
	}
	version := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version"))
	if modern {
		if version != ModernProtocol || r.Header.Get("Mcp-Method") != request.Method {
			return ErrInvalid
		}
		var params map[string]any
		if json.Unmarshal(request.Params, &params) != nil {
			return ErrInvalid
		}
		meta, _ := params["_meta"].(map[string]any)
		if meta["io.modelcontextprotocol/protocolVersion"] != ModernProtocol {
			return ErrInvalid
		}
		if _, ok := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any); !ok {
			return ErrInvalid
		}
		if request.Method == "tools/call" {
			name, _ := params["name"].(string)
			if name == "" || r.Header.Get("Mcp-Name") != name {
				return ErrInvalid
			}
		}
		return nil
	}
	if version != "" && !supportedLegacy(version) {
		return ErrInvalid
	}
	return nil
}

func (h *Handler) discoveryResult() map[string]any {
	return map[string]any{
		"resultType": "complete", "supportedVersions": []string{ModernProtocol},
		"capabilities": map[string]any{"tools": map[string]bool{"listChanged": false}},
		"instructions": "Use read-only MindCreek tools only for knowledge in the effective authorized scope.",
		"ttlMs":        300000, "cacheScope": "private",
	}
}

func (h *Handler) toolList(modern bool) map[string]any {
	result := map[string]any{"tools": toolDefinitions()}
	if modern {
		result["resultType"] = "complete"
		result["ttlMs"] = 300000
		result["cacheScope"] = "private"
	}
	return result
}

func toolDefinitions() []map[string]any {
	readOnly := map[string]bool{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}
	scopeProperty := map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "maxItems": 64, "description": "Optional explicit KB IDs; omitted uses owned, shared, and subscribed knowledge."}
	return []map[string]any{
		{"name": "ask_knowledge_agent", "description": "Ask MindCreek for a grounded answer over an authorized default or explicit KB scope.", "inputSchema": objectSchema(map[string]any{"query": map[string]any{"type": "string", "maxLength": 8000}, "knowledge_base_ids": scopeProperty, "session_id": map[string]any{"type": "string", "maxLength": 128}, "agent_id": map[string]any{"type": "string", "maxLength": 128}}, []string{"query"}), "annotations": readOnly},
		{"name": "get_source_excerpt", "description": "Read a bounded excerpt from a currently authorized citation chunk.", "inputSchema": objectSchema(map[string]any{"chunk_id": map[string]any{"type": "string", "maxLength": 128}, "max_chars": map[string]any{"type": "integer", "minimum": 1, "maximum": 12000}}, []string{"chunk_id"}), "annotations": readOnly},
		{"name": "list_knowledge_bases", "description": "List KBs in the caller's default agent scope.", "inputSchema": objectSchema(map[string]any{}, nil), "annotations": readOnly},
		{"name": "list_publications", "description": "List internal publications visible to the caller.", "inputSchema": objectSchema(map[string]any{"query": map[string]any{"type": "string", "maxLength": 160}, "tag": map[string]any{"type": "string", "maxLength": 40}}, nil), "annotations": readOnly},
		{"name": "list_subscriptions", "description": "List the caller's active MindCreek subscriptions.", "inputSchema": objectSchema(map[string]any{}, nil), "annotations": readOnly},
		{"name": "search_knowledge", "description": "Search authorized MindCreek knowledge without generating an answer.", "inputSchema": objectSchema(map[string]any{"query": map[string]any{"type": "string", "maxLength": 4000}, "knowledge_base_ids": scopeProperty, "knowledge_ids": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "maxItems": 64}}, []string{"query"}), "annotations": readOnly},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func completeResult(value map[string]any) map[string]any {
	value["resultType"] = "complete"
	return value
}

func supportedLegacy(version string) bool {
	switch version {
	case "2025-03-26", "2025-06-18", LegacyProtocol:
		return true
	default:
		return false
	}
}

func toolRPCError(err error) *rpcError {
	switch {
	case errors.Is(err, ErrInvalid):
		return &rpcError{Code: -32602, Message: "Invalid tool arguments"}
	case errors.Is(err, ErrDenied), errors.Is(err, ErrNotFound):
		return &rpcError{Code: -32004, Message: "Resource not found"}
	default:
		return &rpcError{Code: -32000, Message: "Tool operation unavailable"}
	}
}

func correlationID(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if value == "" || len(value) > 128 {
		return "mcp-request"
	}
	return value
}

func validOrigin(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" {
		return true
	}
	origin, err := url.Parse(raw)
	return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") && strings.EqualFold(origin.Host, r.Host)
}

func writeRPC(w http.ResponseWriter, status int, response rpcResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func writeHTTPError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

type fixedWindow struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]bucket
}

type bucket struct {
	start time.Time
	count int
}

func NewFixedWindowLimiter(limit int, window time.Duration) (Limiter, error) {
	if limit < 1 || window <= 0 {
		return nil, fmt.Errorf("positive MCP rate limit and window are required")
	}
	return &fixedWindow{limit: limit, window: window, buckets: make(map[string]bucket)}, nil
}

func (l *fixedWindow) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.buckets[key]
	if current.start.IsZero() || now.Sub(current.start) >= l.window {
		l.buckets[key] = bucket{start: now, count: 1}
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.buckets[key] = current
	return true
}
