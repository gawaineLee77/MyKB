package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
)

const Prefix = "/api/v1/mindcreek/assistant/"

type Channel struct {
	ID                 string   `json:"id"`
	TenantID           uint64   `json:"tenant_id"`
	AgentID            string   `json:"agent_id"`
	AgentMode          string   `json:"agent_mode,omitempty"`
	Name               string   `json:"name"`
	Enabled            bool     `json:"enabled"`
	AllowedOrigins     []string `json:"allowed_origins"`
	WelcomeMessage     string   `json:"welcome_message"`
	RateLimitPerMinute int      `json:"rate_limit_per_minute"`
	RateLimitPerDay    int      `json:"rate_limit_per_day"`
	AllowFileUpload    bool     `json:"allow_file_upload"`
}
type channelInput struct {
	Name               string   `json:"name"`
	Enabled            *bool    `json:"enabled"`
	AllowedOrigins     []string `json:"allowed_origins"`
	WelcomeMessage     string   `json:"welcome_message"`
	RateLimitPerMinute int      `json:"rate_limit_per_minute"`
	RateLimitPerDay    int      `json:"rate_limit_per_day"`
	AllowFileUpload    *bool    `json:"allow_file_upload"`
}
type Service struct {
	Enabled    bool
	Origin     string
	Principals nativeaccess.PrincipalResolver
	Scopes     *nativeaccess.ScopeService
	Models     *nativeaccess.ModelPolicy
	Store      Store
	Employee   func(context.Context, http.Header) error
}
type channelContext struct {
	Channel Channel
	Origin  string
	Actor   nativeaccess.Actor
	Scope   []string
}
type contextKey struct{}

func fail(status int, code string) error { return &nativeaccess.Error{Status: status, Code: code} }

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// Explicit origins only; no wildcard, credentials, paths, encoded host or CSP syntax.
func ValidOrigin(origin string) bool {
	if origin == "" || len(origin) > 512 || strings.ContainsAny(origin, "\r\n\t ;,'\"\\%*(){}") {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.String() != origin {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || strings.HasSuffix(u.Hostname(), ".localhost") || ip != nil && ip.IsLoopback())
}
func (s *Service) FramePolicy(w http.ResponseWriter, r *http.Request) {
	if !s.Enabled {
		// nginx auth_request understands 2xx/401/403; do not turn a disabled
		// feature into an opaque subrequest 500 on the iframe document.
		nativeaccess.WriteError(w, r, fail(403, "feature.disabled"))
		return
	}
	u, err := url.ParseRequestURI(r.Header.Get("X-Original-URI"))
	if err != nil || !strings.HasPrefix(u.Path, "/assistant/") {
		nativeaccess.WriteError(w, r, fail(403, "assistant.origin_denied"))
		return
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/assistant/"), "/")
	origin := u.Query().Get("host_origin")
	if origin == "" {
		origin = s.Origin
	}
	if len(parts) != 2 || !identifier.MatchString(parts[0]) || !identifier.MatchString(parts[1]) || !ValidOrigin(origin) {
		nativeaccess.WriteError(w, r, fail(403, "assistant.origin_denied"))
		return
	}
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' "+origin+"; object-src 'none'; base-uri 'self'")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Service) data(ctx context.Context, method, path string, h http.Header, input any, out any) error {
	raw, err := s.Scopes.Upstream.NativeRequest(ctx, method, path, nil, h, input)
	if err != nil {
		return err
	}
	if method == "DELETE" {
		return nil
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Success || json.Unmarshal(envelope.Data, out) != nil {
		return fail(502, "upstream.invalid_response")
	}
	return nil
}
func (s *Service) channel(ctx context.Context, id string, h http.Header) (Channel, error) {
	var ch Channel
	if !identifier.MatchString(id) {
		return ch, fail(400, "assistant.invalid_channel")
	}
	err := s.data(ctx, "GET", "/api/v1/embed-channels/"+id, h, nil, &ch)
	if err == nil && (ch.ID != id || ch.TenantID == 0 || !identifier.MatchString(ch.AgentID)) {
		err = fail(502, "upstream.invalid_response")
	}
	if err == nil {
		var agent struct {
			Config struct {
				Mode string `json:"agent_mode"`
			} `json:"config"`
		}
		err = s.data(ctx, "GET", "/api/v1/agents/"+ch.AgentID, h, nil, &agent)
		ch.AgentMode = agent.Config.Mode
	}
	return ch, err
}
func (s *Service) validate(ctx context.Context, ch Channel, origin string, actor nativeaccess.Actor, h http.Header) ([]string, error) {
	if !ch.Enabled || ch.TenantID != actor.TenantID {
		return nil, fail(403, "assistant.channel_unavailable")
	}
	allowed := false
	for _, v := range ch.AllowedOrigins {
		if ValidOrigin(v) && v == origin {
			allowed = true
		}
	}
	if !allowed || !ValidOrigin(origin) {
		return nil, fail(403, "assistant.origin_denied")
	}
	return s.Scopes.Resolve(ctx, nil, false, ch.AgentID, actor, h)
}

// This hook also protects main-site/MCP reads of an employee-channel session.
func (s *Service) GuardSession(ctx context.Context, id string, actor nativeaccess.Actor, h http.Header) error {
	b, found, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	c, ok := ctx.Value(contextKey{}).(*channelContext)
	if !s.Enabled || !ok || c.Actor != actor || b.ChannelID != c.Channel.ID || b.AgentID != c.Channel.AgentID || b.HostOrigin != c.Origin {
		return fail(403, "assistant.channel_required")
	}
	return nil
}
func jsonBody(w http.ResponseWriter, r *http.Request, out any, limit int64) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return fail(400, "assistant.invalid_request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fail(400, "assistant.invalid_request")
	}
	return nil
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}
func (s *Service) Handler(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.Enabled {
			nativeaccess.WriteError(w, r, fail(404, "feature.disabled"))
			return
		}
		if r.Header.Get("X-API-Key") != "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			nativeaccess.WriteError(w, r, fail(401, "auth.human_required"))
			return
		}
		actor, err := nativeaccess.ResolveActor(r.Context(), s.Principals, r.Header)
		if err != nil {
			nativeaccess.WriteError(w, r, err)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, Prefix), "/")
		if len(parts) >= 2 && (parts[0] == "agents" || parts[0] == "channels") {
			err = s.manage(w, r, parts, actor)
		} else {
			err = s.consume(w, r, parts, actor, proxy)
		}
		if err != nil {
			nativeaccess.WriteError(w, r, err)
		}
	})
}
func (s *Service) manage(w http.ResponseWriter, r *http.Request, p []string, actor nativeaccess.Actor) error {
	var path string
	switch {
	case len(p) == 3 && p[0] == "agents" && p[2] == "channels" && identifier.MatchString(p[1]) && (r.Method == "GET" || r.Method == "POST"):
		path = "/api/v1/agents/" + p[1] + "/embed-channels"
	case len(p) == 2 && p[0] == "channels" && identifier.MatchString(p[1]) && (r.Method == "GET" || r.Method == "PUT" || r.Method == "DELETE"):
		path = "/api/v1/embed-channels/" + p[1]
	default:
		return fail(404, "route.unclassified")
	}
	// Native still checks Admin+ for every mutation. Enforce the same human role
	// for the product management UI, including its read side.
	principal, err := s.Principals.CurrentPrincipal(r.Context(), r.Header)
	if err != nil {
		return err
	}
	admin := false
	for _, m := range principal.Memberships {
		if m.TenantID == actor.TenantID && (m.Role == "admin" || m.Role == "owner") {
			admin = true
		}
	}
	if !admin {
		return fail(403, "assistant.admin_required")
	}
	var input any
	if r.Method == "POST" || r.Method == "PUT" {
		var value channelInput
		if err := jsonBody(w, r, &value, 32768); err != nil {
			return err
		}
		if len(value.AllowedOrigins) == 0 || len(value.AllowedOrigins) > 32 || strings.TrimSpace(value.Name) == "" || len(value.Name) > 255 || len(value.WelcomeMessage) > 8000 || value.RateLimitPerMinute < 1 || value.RateLimitPerMinute > 1000 || value.RateLimitPerDay < 1 || value.RateLimitPerDay > 1000000 {
			return fail(400, "assistant.invalid_channel_config")
		}
		for _, origin := range value.AllowedOrigins {
			if !ValidOrigin(origin) {
				return fail(400, "assistant.invalid_origin")
			}
		}
		input = value
	}
	if r.Method == "GET" && p[0] == "agents" {
		var channels []Channel
		if err := s.data(r.Context(), r.Method, path, r.Header, input, &channels); err != nil {
			return err
		}
		if channels == nil {
			channels = []Channel{}
		}
		reply(w, 200, channels)
		return nil
	}
	var ch Channel
	if err := s.data(r.Context(), r.Method, path, r.Header, input, &ch); err != nil {
		return err
	}
	if r.Method != "GET" {
		if err := s.Scopes.Store.Record(r.Context(), actor, "assistant.channel."+strings.ToLower(r.Method), "allowed", "", nil, r.Header.Get("X-Request-ID")); err != nil {
			return err
		}
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	reply(w, status, ch)
	return nil
}
func (s *Service) consume(w http.ResponseWriter, r *http.Request, p []string, actor nativeaccess.Actor, proxy http.Handler) error {
	if s.Employee != nil {
		if err := s.Employee(r.Context(), r.Header); err != nil {
			return err
		}
	}
	if len(p) < 3 {
		return fail(404, "route.unclassified")
	}
	tenant, err := strconv.ParseUint(p[0], 10, 64)
	if err != nil || tenant != actor.TenantID {
		return fail(403, "workspace.denied")
	}
	ch, err := s.channel(r.Context(), p[1], r.Header)
	if err != nil {
		return err
	}
	origin := r.Header.Get("X-MindCreek-Host-Origin")
	scope, err := s.validate(r.Context(), ch, origin, actor, r.Header)
	if err != nil {
		return err
	}
	c := &channelContext{Channel: ch, Origin: origin, Actor: actor, Scope: scope}
	r = r.WithContext(context.WithValue(r.Context(), contextKey{}, c))
	if len(p) == 3 && p[2] == "config" && r.Method == "GET" {
		reply(w, 200, ch)
		return nil
	}
	if len(p) == 3 && p[2] == "sessions" {
		switch r.Method {
		case "POST":
			var input struct {
				Title string `json:"title"`
			}
			if err := jsonBody(w, r, &input, 4096); err != nil {
				return err
			}
			if len(input.Title) > 255 {
				return fail(400, "assistant.invalid_request")
			}
			if err := s.Store.Take(r.Context(), ch.ID, actor.ID, ch.RateLimitPerMinute, ch.RateLimitPerDay); err != nil {
				return err
			}
			var created struct {
				ID string `json:"id"`
			}
			if err := s.data(r.Context(), "POST", "/api/v1/sessions", r.Header, map[string]any{"title": input.Title, "agent_id": ch.AgentID}, &created); err != nil {
				return err
			}
			if !identifier.MatchString(created.ID) {
				return fail(502, "upstream.invalid_response")
			}
			b := Binding{SessionID: created.ID, ChannelID: ch.ID, AgentID: ch.AgentID, HostOrigin: origin}
			if err := s.Store.Create(r.Context(), b, actor); err != nil {
				return err
			}
			reply(w, 201, map[string]string{"id": created.ID})
			return nil
		case "GET":
			rows, err := s.Store.List(r.Context(), ch.ID, actor)
			if err != nil {
				return err
			}
			visible := []Binding{}
			for _, b := range rows {
				if b.HostOrigin != origin || b.AgentID != ch.AgentID {
					continue
				}
				if _, err := s.Scopes.CheckSession(r.Context(), b.SessionID, actor, r.Header); err != nil {
					continue
				}
				visible = append(visible, b)
			}
			reply(w, 200, visible)
			return nil
		}
	}
	if len(p) < 4 || p[2] != "proxy" {
		return fail(404, "route.unclassified")
	}
	nativePath := "/" + strings.Join(p[3:], "/")
	sessionID, kind := proxyRoute(r.Method, nativePath)
	if kind == "" {
		return fail(404, "route.unclassified")
	}
	if err := checkQuery(r.URL.Query(), kind); err != nil {
		return err
	}
	if sessionID == "" {
		sessionID = r.Header.Get("X-MindCreek-Session-ID")
	}
	if !identifier.MatchString(sessionID) {
		return fail(400, "assistant.session_required")
	}
	b, found, err := s.Store.Get(r.Context(), sessionID)
	if err != nil {
		return err
	}
	if !found || b.ChannelID != ch.ID {
		return fail(403, "assistant.session_denied")
	}
	binding, err := s.Scopes.CheckSession(r.Context(), sessionID, actor, r.Header)
	if err != nil {
		return err
	}
	for _, kb := range binding.KBIDs {
		if !includes(scope, kb) {
			return fail(403, "assistant.scope_revoked")
		}
	}
	if kind == "chunk" || kind == "knowledge" || kind == "kbfile" {
		pathParts := strings.Split(nativePath, "/")
		id := pathParts[4]
		if kind == "chunk" {
			id = pathParts[5]
		}
		var kb string
		switch kind {
		case "chunk":
			kb, err = s.Scopes.Upstream.KnowledgeBaseForChunk(r.Context(), id, r.Header)
		case "knowledge":
			kb, err = s.Scopes.Upstream.KnowledgeBaseForKnowledge(r.Context(), id, r.Header)
		case "kbfile":
			kb = id
		}
		if err != nil {
			return err
		}
		if !includes(binding.KBIDs, kb) || !includes(scope, kb) {
			return fail(403, "assistant.reference_denied")
		}
	}
	r.URL.Path = nativePath
	r.URL.RawPath = ""
	r.RequestURI = r.URL.RequestURI()
	if kind == "chat" {
		expected := "/api/v1/agent-chat/" + sessionID
		if ch.AgentMode == "quick-answer" {
			expected = "/api/v1/knowledge-chat/" + sessionID
		}
		if nativePath != expected {
			return fail(400, "assistant.agent_mode_mismatch")
		}
		var body struct {
			Query   string            `json:"query"`
			Images  []json.RawMessage `json:"images,omitempty"`
			Uploads []json.RawMessage `json:"attachment_uploads,omitempty"`
		}
		if err := jsonBody(w, r, &body, 16<<20); err != nil {
			return err
		}
		if !ch.AllowFileUpload && (len(body.Images) > 0 || len(body.Uploads) > 0) {
			return fail(403, "assistant.upload_disabled")
		}
		if err := s.Store.Take(r.Context(), ch.ID, actor.ID, ch.RateLimitPerMinute, ch.RateLimitPerDay); err != nil {
			return err
		}
		// The employee never selects another Agent or expands the server's KB scope.
		rewritten := map[string]any{"query": body.Query, "agent_id": ch.AgentID, "knowledge_base_ids": scope, "channel": "web", "images": body.Images, "attachment_uploads": body.Uploads}
		raw, _ := json.Marshal(rewritten)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		r.ContentLength = int64(len(raw))
		r.Header.Set("Content-Type", "application/json")
	}
	if err := s.Scopes.Check(r.Context(), r, actor); err != nil {
		return err
	}
	if err := s.Models.Check(r.Context(), r, actor); err != nil {
		return err
	}
	proxy.ServeHTTP(w, r)
	return nil
}
func includes(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// Never forward arbitrary selectors or public file-link modes from the iframe.
func checkQuery(q url.Values, kind string) error {
	allowed := map[string]bool{}
	switch kind {
	case "history":
		allowed["limit"], allowed["before_time"] = true, true
	case "stream":
		allowed["message_id"] = true
	case "kbfile", "messagefile":
		allowed["file_path"] = true
	}
	for name, values := range q {
		if !allowed[name] || len(values) != 1 || len(values[0]) > 4096 {
			return fail(400, "assistant.invalid_query")
		}
	}
	if id := q.Get("message_id"); id != "" && !identifier.MatchString(id) {
		return fail(400, "assistant.invalid_query")
	}
	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 100 {
			return fail(400, "assistant.invalid_query")
		}
	}
	return nil
}

var proxyPatterns = []struct {
	method, pattern, kind string
	session               bool
}{
	{"POST", `^/api/v1/(?:knowledge-chat|agent-chat)/([\w-]+)$`, "chat", true},
	{"GET", `^/api/v1/messages/([\w-]+)/load$`, "history", true},
	{"GET", `^/api/v1/sessions/continue-stream/([\w-]+)$`, "stream", true},
	{"POST", `^/api/v1/sessions/([\w-]+)/stop$`, "stop", true},
	{"GET", `^/api/v1/chunks/by-id/([\w-]+)$`, "chunk", false},
	{"GET", `^/api/v1/knowledge/([\w-]+)(?:/preview)?$`, "knowledge", false},
	{"GET", `^/api/v1/knowledge-bases/([\w-]+)/files$`, "kbfile", false},
	{"GET", `^/api/v1/sessions/([\w-]+)/messages/[\w-]+/files$`, "messagefile", true},
}

func proxyRoute(method, path string) (string, string) {
	for _, p := range proxyPatterns {
		if p.method == method {
			m := regexp.MustCompile(p.pattern).FindStringSubmatch(path)
			if m != nil {
				if p.session {
					return m[1], p.kind
				}
				return "", p.kind
			}
		}
	}
	return "", ""
}
