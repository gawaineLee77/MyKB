package kbguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type Authorizer interface {
	Authorize(context.Context, http.Header, string, ...string) (kbacl.Permissions, error)
	AllowedKnowledgeBases(context.Context, http.Header, []string) ([]string, error)
}
type Upstream interface {
	NativeRequest(context.Context, string, string, url.Values, http.Header, any) (json.RawMessage, error)
	KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error)
	KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error)
}
type Guard struct {
	ACL      Authorizer
	Upstream Upstream
	// WithLock must coordinate with membership changes as well as KB ACL writes.
	// The callback covers fresh authorization through native response receipt.
	WithLock          func(context.Context, http.Header, []string, func() error) error
	WithCollaborator  func(context.Context, http.Header, string, []string, func(http.Header) error) error
	BindCreated       func(context.Context, http.Header, string) error
	BeforeCreate      func(context.Context, http.Header, string, string) error
	AfterCreate       func(context.Context, http.Header, string, string) error
	MarkDeleted       func(context.Context, http.Header, string) error
	AuthorizeResource func(context.Context, http.Header, string, string) error
	RewriteResources  func(context.Context, http.Header, any) error
	Audit             func(context.Context, http.Header, AuditEvent) error
}

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string           { return e.Code }
func (e *Error) HTTPStatus() int         { return e.Status }
func (e *Error) PublicCode() string      { return e.Code }
func fail(status int, code string) error { return &Error{status, code} }
func writeError(w http.ResponseWriter, err error) {
	status, code := 503, "kb_acl.authorization_unavailable"
	var public interface {
		HTTPStatus() int
		PublicCode() string
	}
	var native *weknora.Error
	if errors.As(err, &public) {
		status, code = public.HTTPStatus(), public.PublicCode()
	} else if errors.As(err, &native) {
		status = native.StatusCode
		if status < 400 || status > 599 {
			status = 502
		}
		code = "kb_acl.upstream_unavailable"
		if status == 403 || status == 404 {
			status = 404
			code = "kb_acl.not_found"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": code, "code": code})
}

type stateKey struct{}
type state struct {
	route               route
	params              map[string]string
	headers             http.Header
	body                map[string]any
	kbs                 []string
	permissions         map[string]kbacl.Permissions
	mutation            bool
	createKey           string
	responsePermissions map[string]kbacl.Permissions
	task                map[string]any
}

// OriginalHeaders returns caller credentials even when a fixed native write is
// executed with a service credential. Response scope checks must use these.
func OriginalHeaders(r *http.Request) http.Header {
	if s, ok := r.Context().Value(stateKey{}).(*state); ok {
		return s.headers.Clone()
	}
	return r.Header.Clone()
}

func (g *Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		rt, params, ok := classify(rq.Method, rq.URL.Path)
		if !ok {
			next.ServeHTTP(w, rq)
			return
		}
		tracked := &trackedWriter{ResponseWriter: w}
		w = tracked
		if rt.kind == "disabled" {
			writeError(w, fail(409, "kb_acl.endpoint_not_supported"))
			return
		}
		if g.ACL == nil || g.Upstream == nil {
			writeError(w, fail(503, "kb_acl.not_ready"))
			return
		}
		s := &state{route: rt, params: params, headers: rq.Header.Clone(), permissions: map[string]kbacl.Permissions{}, mutation: writeMethod(rq.Method) && rt.kind != "retrieval" && rt.kind != "export" && !strings.HasSuffix(rq.URL.Path, "/hybrid-search") && !strings.HasSuffix(rq.URL.Path, "/faq/search")}
		*rq = *rq.WithContext(context.WithValue(rq.Context(), stateKey{}, s))
		body, err := readBody(rq, rt)
		if err != nil {
			writeError(w, err)
			return
		}
		s.body = body
		if rt.kind == "faq" && body["mode"] == "replace" {
			s.route.actions = append(append([]string(nil), s.route.actions...), "document.delete", "chunk.delete")
		}
		if rt.kind == "create" {
			if forgedIdentity(body) {
				writeError(w, fail(400, "kb_acl.creator_immutable"))
				return
			}
			s.createKey = strings.TrimSpace(rq.Header.Get("Idempotency-Key"))
			if !validID(s.createKey) {
				writeError(w, fail(400, "kb_acl.idempotency_key_required"))
				return
			}
		}
		if err = g.resolve(rq, s); err != nil {
			writeError(w, err)
			return
		}
		execute := func() (err error) {
			// Re-resolve under the membership/ACL lock: a concurrent move or
			// revocation cannot turn a checked parent into an unchecked target.
			if s.mutation {
				if e := g.resolve(rq, s); e != nil {
					return e
				}
			}
			for _, id := range s.kbs {
				p, e := g.ACL.Authorize(rq.Context(), s.headers, id, s.route.actions...)
				if e != nil {
					return e
				}
				s.permissions[id] = p
			}
			if err := g.validateReferences(rq, s); err != nil {
				return err
			}
			if s.route.kind == "faq" {
				if err := g.validateFAQ(rq, s); err != nil {
					return err
				}
			}
			if len(s.kbs) > 0 && (s.mutation || contains(s.route.actions, "document.download") || contains(s.route.actions, "chunk.export")) {
				if g.Audit == nil {
					return fail(503, "kb_acl.audit_unavailable")
				}
				event, e := auditEvent(rq, s)
				if e != nil {
					return e
				}
				event.Outcome = "attempt"
				if e = g.Audit(rq.Context(), s.headers, event); e != nil {
					return e
				}
				defer func() {
					event.Status = tracked.status
					event.Outcome = "accepted"
					if err != nil || tracked.status >= 400 {
						event.Outcome = "failed"
						if event.Status == 0 {
							event.Status = 503
							var public interface{ HTTPStatus() int }
							if errors.As(err, &public) {
								event.Status = public.HTTPStatus()
							}
						}
					}
					if e := g.Audit(context.WithoutCancel(rq.Context()), s.headers, event); err == nil && e != nil {
						err = e
					}
				}()
			}
			if rt.kind == "create" {
				if g.BeforeCreate == nil || g.AfterCreate == nil {
					return fail(503, "kb_acl.creation_binding_unavailable")
				}
				canonical, e := json.Marshal(s.body)
				if e != nil {
					return e
				}
				digest := sha256.Sum256(canonical)
				if e := g.BeforeCreate(rq.Context(), s.headers, s.createKey, hex.EncodeToString(digest[:])); e != nil {
					return e
				}
			}
			if rt.kind == "file" {
				check := g.AuthorizeResource
				if check == nil {
					check = g.VerifyResource
				}
				if err := check(rq.Context(), s.headers, params["kb"], rq.URL.Query().Get("file_path")); err != nil {
					return err
				}
			}
			if rt.kind == "empty" {
				writeJSON(w, map[string]any{"success": true, "data": []any{}, "total": 0})
				return nil
			}
			if rt.kind == "task" {
				if e := g.filterValue(rq, s, s.task, s.kbs[0]); e != nil {
					return e
				}
				writeJSON(w, map[string]any{"success": true, "data": s.task})
				return nil
			}
			if rt.kind == "search" {
				return g.search(w, rq, s)
			}
			if rt.kind == "export" {
				return g.export(w, rq, s)
			}
			if s.mutation && len(s.kbs) == 1 && !s.permissions[s.kbs[0]].Manage {
				if err := validateFields(s); err != nil {
					return err
				}
				// Machine callers retain their original native capability checks;
				// never exchange a public API key for a more powerful service key.
				if s.headers.Get("X-API-Key") != "" {
					next.ServeHTTP(w, rq)
					return tracked.outcome()
				}
				if g.WithCollaborator == nil {
					return fail(503, "kb_acl.collaboration_unavailable")
				}
				return g.WithCollaborator(rq.Context(), s.headers, s.kbs[0], s.route.actions, func(h http.Header) error {
					forward := rq.Clone(rq.Context())
					forward.Header = rq.Header.Clone()
					for _, name := range []string{"Authorization", "X-API-Key", "X-Tenant-ID", "X-External-User-ID", "X-External-User-Token", "Cookie"} {
						forward.Header.Del(name)
					}
					for name, values := range h {
						forward.Header[name] = append([]string(nil), values...)
					}
					next.ServeHTTP(w, forward)
					return tracked.outcome()
				})
			}
			next.ServeHTTP(w, rq)
			return tracked.outcome()
		}
		if s.mutation {
			if g.WithLock == nil {
				writeError(w, fail(503, "kb_acl.lock_unavailable"))
				return
			}
			err = g.WithLock(rq.Context(), s.headers, s.kbs, execute)
		} else {
			err = execute()
		}
		if err != nil && !tracked.committed {
			writeError(w, err)
		}
	})
}

// ResponseController follows Unwrap for streaming; status is also reported to
// delegation/audit callbacks without writing a second error after commitment.
type trackedWriter struct {
	http.ResponseWriter
	committed bool
	status    int
}

func (w *trackedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *trackedWriter) WriteHeader(status int) {
	if w.committed {
		return
	}
	w.committed = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *trackedWriter) Write(p []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
func (w *trackedWriter) Flush() {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *trackedWriter) outcome() error {
	if w.status >= 400 {
		return fail(w.status, "kb_acl.native_rejected")
	}
	return nil
}

func (g *Guard) resolve(rq *http.Request, s *state) error {
	ids := []string{}
	if s.route.kind == "task" {
		data, err := g.object(rq.Context(), "/api/v1/faq/import/progress/"+url.PathEscape(s.params["task"]), nil, s.headers)
		if err != nil {
			return err
		}
		kb, _ := data["kb_id"].(string)
		knowledge, _ := data["knowledge_id"].(string)
		if data["task_id"] != s.params["task"] || !validID(kb) || !validID(knowledge) {
			return fail(409, "kb_acl.task_binding_unavailable")
		}
		parent, err := g.Upstream.KnowledgeBaseForKnowledge(rq.Context(), knowledge, s.headers)
		if err != nil {
			return err
		}
		if parent != kb {
			return fail(403, "kb_acl.parent_mismatch")
		}
		s.task = data
		ids = append(ids, kb)
	}
	if id := s.params["kb"]; id != "" {
		ids = append(ids, id)
	}
	knowledge := []string{}
	if id := s.params["knowledge"]; id != "" {
		knowledge = append(knowledge, id)
	}
	if s.route.kind == "batch-read" {
		knowledge = append(knowledge, rq.URL.Query()["ids"]...)
	}
	if s.route.kind == "batch" {
		knowledge = append(knowledge, stringList(s.body["ids"])...)
		knowledge = append(knowledge, stringList(s.body["knowledge_ids"])...)
	}
	if s.route.kind == "tags" {
		if updates, ok := s.body["updates"].(map[string]any); ok {
			for id := range updates {
				knowledge = append(knowledge, id)
			}
		}
	}
	if s.route.kind == "export" {
		knowledge = append(knowledge, stringList(s.body["knowledge_id"])...)
		knowledge = append(knowledge, stringList(s.body["knowledge_ids"])...)
		if id := rq.URL.Query().Get("knowledge_id"); id != "" {
			knowledge = append(knowledge, id)
		}
	}
	if id, ok := s.body["kb_id"].(string); ok {
		ids = append(ids, id)
	}
	if id := rq.URL.Query().Get("kb_id"); id != "" {
		ids = append(ids, id)
	}
	if (s.route.kind == "batch" || s.route.kind == "batch-read" || s.route.kind == "tags" || s.route.kind == "export") && len(knowledge) == 0 {
		return fail(400, "kb_acl.target_required")
	}
	if len(knowledge) > 200 {
		return fail(413, "kb_acl.too_many_targets")
	}
	var knowledgeKB string
	for _, id := range unique(knowledge) {
		if !validID(id) {
			return fail(400, "kb_acl.invalid_id")
		}
		kb, err := g.Upstream.KnowledgeBaseForKnowledge(rq.Context(), id, s.headers)
		if err != nil {
			return err
		}
		if knowledgeKB != "" && knowledgeKB != kb && s.route.kind != "batch-read" {
			return fail(403, "kb_acl.parent_mismatch")
		}
		knowledgeKB = kb
		ids = append(ids, kb)
	}
	if id := s.params["chunk"]; id != "" {
		chunk, err := g.object(rq.Context(), "/api/v1/chunks/by-id/"+url.PathEscape(id), nil, s.headers)
		if err != nil {
			return err
		}
		kb, _ := chunk["knowledge_base_id"].(string)
		parent, _ := chunk["knowledge_id"].(string)
		if !validID(kb) || s.params["knowledge"] != "" && parent != s.params["knowledge"] {
			return fail(403, "kb_acl.parent_mismatch")
		}
		ids = append(ids, kb)
	}
	ids = unique(ids)
	for _, id := range ids {
		if !validID(id) {
			return fail(400, "kb_acl.invalid_id")
		}
	}
	if (s.route.kind == "batch" || s.route.kind == "tags" || s.route.kind == "export" || s.params["knowledge"] != "") && len(ids) != 1 {
		return fail(403, "kb_acl.parent_mismatch")
	}
	if s.mutation && len(s.kbs) > 0 && !sameIDs(s.kbs, ids) {
		return fail(409, "kb_acl.parent_changed")
	}
	s.kbs = ids
	return nil
}

func (g *Guard) object(ctx context.Context, path string, q url.Values, h http.Header) (map[string]any, error) {
	envelope, err := g.envelope(ctx, path, q, h)
	if err != nil {
		return nil, err
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return nil, fail(502, "kb_acl.invalid_upstream_response")
	}
	return data, nil
}
func (g *Guard) envelope(ctx context.Context, path string, q url.Values, h http.Header) (map[string]any, error) {
	raw, err := g.Upstream.NativeRequest(ctx, "GET", path, q, h, nil)
	if err != nil {
		return nil, err
	}
	var e map[string]any
	if json.Unmarshal(raw, &e) != nil || e["success"] != true {
		return nil, fail(502, "kb_acl.invalid_upstream_response")
	}
	return e, nil
}
func validID(id string) bool {
	return id != "" && len(id) <= 128 && id != "." && id != ".." && !strings.ContainsAny(id, "/\\%\x00\r\n\t ,?#")
}
func unique(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func sameIDs(a, b []string) bool {
	return strings.Join(unique(a), "\x00") == strings.Join(unique(b), "\x00")
}
func stringList(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []string:
		return x
	case []any:
		var out []string
		for _, v := range x {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
