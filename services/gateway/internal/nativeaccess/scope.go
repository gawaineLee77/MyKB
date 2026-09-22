package nativeaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const MaxScope = 256

type NativeReader interface {
	NativeRequest(context.Context, string, string, url.Values, http.Header, any) (json.RawMessage, error)
	KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error)
	KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error)
	ValidateSession(context.Context, string, http.Header) error
}

type ScopeService struct {
	Upstream     NativeReader
	Store        SessionStore
	SessionGuard func(context.Context, string, Actor, http.Header) error
}

func (s *ScopeService) AgentConfig(ctx context.Context, id string, h http.Header) (map[string]any, error) {
	if !validID(id) {
		return nil, &Error{400, "scope.invalid_id"}
	}
	value, err := s.agent(ctx, id, h)
	if err != nil {
		return nil, err
	}
	agent, ok := value.(map[string]any)
	if !ok {
		return nil, &Error{502, "upstream.invalid_response"}
	}
	config, ok := agent["config"].(map[string]any)
	if !ok {
		return nil, &Error{502, "upstream.invalid_response"}
	}
	return config, nil
}

type requestState struct {
	Actor       Actor
	KBIDs       []string
	SessionIDs  []string
	Create      bool
	StrictScope bool
}
type scopeContextKey struct{}

func readObject(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return map[string]any{}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20+1))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) > 16<<20 {
		return nil, &Error{413, "request.body_too_large"}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil || object == nil {
		return nil, &Error{400, "request.invalid_json"}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, &Error{400, "request.invalid_json"}
	}
	return object, nil
}

func replaceObject(r *http.Request, body map[string]any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	r.Header.Set("Content-Type", "application/json")
	return nil
}

func (s *ScopeService) data(ctx context.Context, path string, query url.Values, h http.Header) (any, error) {
	raw, err := s.Upstream.NativeRequest(ctx, http.MethodGet, path, query, h, nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil || !envelope.Success {
		return nil, &Error{502, "upstream.invalid_response"}
	}
	return envelope.Data, nil
}

func (s *ScopeService) CheckKB(ctx context.Context, id string, h http.Header) error {
	if !validID(id) {
		return &Error{400, "scope.invalid_id"}
	}
	value, err := s.data(ctx, "/api/v1/knowledge-bases/"+url.PathEscape(id), nil, h)
	if err != nil {
		return err
	}
	kb, ok := value.(map[string]any)
	if !ok || kb["id"] != id {
		return &Error{502, "upstream.invalid_response"}
	}
	return nil
}

func (s *ScopeService) listIDs(ctx context.Context, path string, query url.Values, h http.Header) ([]string, error) {
	value, err := s.data(ctx, path, query, h)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return []string{}, nil
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, &Error{502, "upstream.invalid_response"}
	}
	ids := []string{}
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		if nested, ok := object["knowledge_base"].(map[string]any); ok {
			object = nested
		}
		if id, ok := object["id"].(string); ok && validID(id) {
			ids = append(ids, id)
		} else {
			return nil, &Error{502, "upstream.invalid_response"}
		}
	}
	return unique(ids), nil
}

func (s *ScopeService) Defaults(ctx context.Context, h http.Header) ([]string, error) {
	ids, err := s.listIDs(ctx, "/api/v1/knowledge-bases", nil, h)
	if err != nil {
		return nil, err
	}
	shared, err := s.listIDs(ctx, "/api/v1/shared-knowledge-bases", nil, h)
	// Native tenant keys without manage_spaces cannot enumerate organization
	// sharing. Their tenant-local list is already filtered by native KB scope.
	var upstreamErr *weknora.Error
	if err != nil && !(h.Get("X-API-Key") != "" && errors.As(err, &upstreamErr) && upstreamErr.StatusCode == 403) {
		return nil, err
	}
	ids = unique(append(ids, shared...))
	if len(ids) > MaxScope {
		return nil, &Error{422, "scope.too_large"}
	}
	return s.available(ctx, ids, h)
}

func (s *ScopeService) available(ctx context.Context, ids []string, h http.Header) ([]string, error) {
	result := []string{}
	for _, id := range unique(ids) {
		if err := s.CheckKB(ctx, id, h); err != nil {
			var e *weknora.Error
			if errors.As(err, &e) && (e.StatusCode == 403 || e.StatusCode == 404) {
				continue
			}
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

// Resolve rejects the entire explicit request before any retrieval. For default
// selection it uses only resources still accessible with these exact credentials.
func (s *ScopeService) Resolve(ctx context.Context, ids []string, explicit bool, agentID string, actor Actor, h http.Header) ([]string, error) {
	ids = unique(ids)
	if len(ids) > MaxScope {
		return nil, &Error{422, "scope.too_large"}
	}
	for _, id := range ids {
		if err := s.CheckKB(ctx, id, h); err != nil {
			return nil, err
		}
	}
	var allowed []string
	if agentID != "" {
		if !validID(agentID) {
			return nil, &Error{400, "scope.invalid_id"}
		}
		value, err := s.agent(ctx, agentID, h)
		if err != nil {
			return nil, err
		}
		agent, ok := value.(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		config, ok := agent["config"].(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		if config["memory_enabled"] == true {
			return nil, &Error{403, "feature.disabled"}
		}
		if config["agent_mode"] != "quick-answer" {
			tools := stringsValue(config["allowed_tools"])
			if len(tools) == 0 {
				return nil, &Error{409, "agent.tools_scope_unavailable"}
			}
			for _, tool := range tools {
				if tool == "search_conversations" {
					return nil, &Error{409, "history.vector_scope_unavailable"}
				}
			}
		}
		mode, _ := config["kb_selection_mode"].(string)
		switch mode {
		case "none":
			allowed = []string{}
		case "all":
			if tenant := uintValue(agent["tenant_id"]); tenant != 0 && tenant != actor.TenantID {
				allowed, err = s.listIDs(ctx, "/api/v1/knowledge-bases", url.Values{"agent_id": {agentID}}, h)
			} else {
				allowed, err = s.Defaults(ctx, h)
			}
		default:
			allowed = stringsValue(config["knowledge_bases"])
		}
		if err != nil {
			return nil, err
		}
		allowed, err = s.available(ctx, allowed, h)
		if err != nil {
			return nil, err
		}
		if mode == "all" {
			allowed, err = s.compatible(ctx, allowed, config, h)
			if err != nil {
				return nil, err
			}
		}
		if explicit && !subset(ids, allowed) {
			return nil, &Error{403, "agent.scope_denied"}
		}
		if !explicit {
			if only, _ := config["retrieve_kb_only_when_mentioned"].(bool); only {
				return []string{}, nil
			}
			ids = allowed
		}
		if mode == "none" {
			return ids, nil
		}
	} else if !explicit {
		var err error
		ids, err = s.Defaults(ctx, h)
		if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, &Error{403, "scope.empty"}
	}
	return unique(ids), nil
}

func (s *ScopeService) CheckSession(ctx context.Context, id string, actor Actor, h http.Header) (Binding, error) {
	binding, err := s.Store.Binding(ctx, id)
	if err != nil {
		return Binding{}, err
	}
	if binding.Actor != actor {
		return Binding{}, &Error{403, "session.principal_mismatch"}
	}
	if s.SessionGuard != nil {
		if err := s.SessionGuard(ctx, id, actor, h); err != nil {
			return Binding{}, err
		}
	}
	if err := s.Upstream.ValidateSession(ctx, id, h); err != nil {
		return Binding{}, err
	}
	for _, kb := range binding.KBIDs {
		if err := s.CheckKB(ctx, kb, h); err != nil {
			return Binding{}, err
		}
	}
	return binding, nil
}

func (s *ScopeService) Check(ctx context.Context, r *http.Request, actor Actor) (err error) {
	state := &requestState{Actor: actor, Create: r.Method == "POST" && r.URL.Path == "/api/v1/sessions"}
	*r = *r.WithContext(context.WithValue(r.Context(), scopeContextKey{}, state))
	defer func() {
		outcome, code := "allowed", ""
		if err != nil {
			outcome = "denied"
			code = "scope.denied"
			var e *Error
			if errors.As(err, &e) {
				code = e.Code
			}
		}
		// Never log query text, documents, request bodies or credentials.
		if auditErr := s.Store.Record(ctx, actor, "web.scope", outcome, code, state.KBIDs, r.Header.Get("X-Request-ID")); auditErr != nil {
			err = auditErr
		}
	}()
	body, err := readObject(r)
	if err != nil {
		return err
	}
	refs := references{}
	collect(body, &refs)
	if r.URL.Path == "/api/v1/messages/search" {
		// Native v0.8.0 filters vector history hits by session only after KB
		// retrieval. Until it accepts a pre-retrieval owner/session predicate,
		// only the native keyword path can satisfy the R3 isolation boundary.
		if body["mode"] != "keyword" {
			return &Error{409, "history.vector_scope_unavailable"}
		}
		if len(refs.sessions) == 0 {
			bindings, e := s.Store.Bindings(ctx, actor)
			if e != nil {
				return e
			}
			for _, binding := range bindings {
				if _, e := s.CheckSession(ctx, binding.SessionID, actor, r.Header); e != nil {
					if denied(e) {
						continue
					}
					return e
				}
				refs.sessions = append(refs.sessions, binding.SessionID)
			}
			if len(refs.sessions) == 0 {
				return &Error{403, "session.scope_empty"}
			}
			body["session_ids"] = refs.sessions
			if e := replaceObject(r, body); e != nil {
				return e
			}
		}
	}
	for key, values := range r.URL.Query() {
		for _, value := range values {
			collect(map[string]any{key: value}, &refs)
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 4 {
		switch parts[2] {
		case "knowledge-bases":
			if strings.HasSuffix(r.URL.Path, "/hybrid-search") {
				refs.kbs = append(refs.kbs, parts[3])
				state.KBIDs = []string{parts[3]}
				state.StrictScope = true
			}
		case "sessions":
			if parts[3] == "continue-stream" && len(parts) >= 5 {
				refs.sessions = append(refs.sessions, parts[4])
			} else if parts[3] != "batch" {
				refs.sessions = append(refs.sessions, parts[3])
			}
		case "knowledge-chat", "agent-chat":
			refs.sessions = append(refs.sessions, parts[3])
		case "messages":
			if parts[3] != "search" && parts[3] != "chat-history-stats" {
				refs.sessions = append(refs.sessions, parts[3])
			}
		}
	}
	if r.URL.Path == "/api/v1/sessions/batch" {
		refs.sessions = append(refs.sessions, stringsValue(body["ids"])...)
	}
	for _, id := range unique(refs.sessions) {
		binding, err := s.CheckSession(ctx, id, actor, r.Header)
		if err != nil {
			return err
		}
		state.SessionIDs = append(state.SessionIDs, id)
		state.KBIDs = append(state.KBIDs, binding.KBIDs...)
		state.StrictScope = true
	}
	for _, id := range unique(refs.knowledge) {
		kb, err := s.Upstream.KnowledgeBaseForKnowledge(ctx, id, r.Header)
		if err != nil {
			return err
		}
		refs.kbs = append(refs.kbs, kb)
	}
	for _, id := range unique(refs.chunks) {
		kb, err := s.Upstream.KnowledgeBaseForChunk(ctx, id, r.Header)
		if err != nil {
			return err
		}
		refs.kbs = append(refs.kbs, kb)
	}
	for _, id := range unique(refs.kbs) {
		if err := s.CheckKB(ctx, id, r.Header); err != nil {
			return err
		}
	}
	retrieval := r.Method == "POST" && (r.URL.Path == "/api/v1/knowledge-search" || strings.HasPrefix(r.URL.Path, "/api/v1/knowledge-chat/") || strings.HasPrefix(r.URL.Path, "/api/v1/agent-chat/"))
	if retrieval {
		state.StrictScope = true
		if len(unique(refs.agents)) > 1 {
			return &Error{400, "agent.ambiguous"}
		}
		agentID := ""
		if len(refs.agents) > 0 {
			agentID = refs.agents[0]
		}
		if source := uintValue(body["agent_source_tenant_id"]); source != 0 && agentID != "" {
			value, e := s.agent(ctx, agentID, r.Header)
			if e != nil {
				return e
			}
			metadata, ok := value.(map[string]any)
			// The public metadata API has no source-tenant disambiguator.
			// Never preflight one same-ID Agent and execute a different one.
			if !ok || uintValue(metadata["tenant_id"]) != source {
				return &Error{409, "agent.source_ambiguous"}
			}
		}
		state.KBIDs, err = s.Resolve(ctx, refs.kbs, len(refs.kbs) > 0, agentID, actor, r.Header)
		if err != nil {
			return err
		}
		body["knowledge_base_ids"] = state.KBIDs
		delete(body, "knowledge_base_id")
		if err := replaceObject(r, body); err != nil {
			return err
		}
		for _, id := range state.SessionIDs {
			if err := s.Store.Bind(ctx, id, actor, state.KBIDs); err != nil {
				return err
			}
		}
	}
	return nil
}

type references struct{ kbs, knowledge, chunks, sessions, agents []string }

func collect(value any, refs *references) {
	switch object := value.(type) {
	case []any:
		for _, item := range object {
			collect(item, refs)
		}
	case map[string]any:
		if id, ok := object["id"].(string); ok {
			switch object["type"] {
			case "kb", "knowledge_base":
				refs.kbs = append(refs.kbs, id)
			case "file":
				refs.knowledge = append(refs.knowledge, id)
			}
		}
		for key, item := range object {
			switch key {
			case "knowledge_base_id", "knowledge_base_ids", "knowledge_bases", "kb_id":
				refs.kbs = append(refs.kbs, stringsValue(item)...)
			case "knowledge_id", "knowledge_ids":
				refs.knowledge = append(refs.knowledge, stringsValue(item)...)
			case "chunk_id", "chunk_ids":
				refs.chunks = append(refs.chunks, stringsValue(item)...)
			case "session_id", "session_ids":
				refs.sessions = append(refs.sessions, stringsValue(item)...)
			case "agent_id":
				refs.agents = append(refs.agents, stringsValue(item)...)
			}
			collect(item, refs)
		}
	}
}
func stringsValue(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []any:
		out := []string{}
		for _, v := range x {
			if text, ok := v.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}
func validID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/?#\\\x00") && id != "." && id != ".."
}
func unique(ids []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func subset(ids, allowed []string) bool {
	set := map[string]bool{}
	for _, id := range allowed {
		set[id] = true
	}
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}
func upstreamDenied(err error) bool {
	var e *weknora.Error
	return errors.As(err, &e) && (e.StatusCode == 403 || e.StatusCode == 404)
}

func uintValue(value any) uint64 {
	switch n := value.(type) {
	case json.Number:
		id, _ := strconv.ParseUint(string(n), 10, 64)
		return id
	case float64:
		return uint64(n)
	}
	return 0
}
