// Package graphview is a read-only adapter for the pinned native graph schema.
// Native HTTP APIs remain authoritative for identity and resource visibility.
package graphview

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const Route = "GET /api/v1/mindcreek/graph/knowledge-bases/{kb_id}"

type Node struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	KnowledgeID string   `json:"knowledge_id"`
	Attributes  []string `json:"attributes"`
}
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}
type Graph struct {
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges"`
	Truncated bool   `json:"truncated"`
	State     string `json:"state"`
}
type Query struct {
	KBID, Search, Relation, NodeID string
	Limit                          int
}
type Reader interface {
	Read(context.Context, Query) (Graph, error)
}
type Upstream interface {
	nativeaccess.PrincipalResolver
	NativeRequest(context.Context, string, string, url.Values, http.Header, any) (json.RawMessage, error)
	KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error)
}
type Service struct {
	Enabled         bool
	Upstream        Upstream
	Reader          Reader
	KnowledgeAccess func(context.Context, string, http.Header) error
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func fail(status int, code string) error { return &nativeaccess.Error{Status: status, Code: code} }

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	graph, err := s.read(ctx, r)
	if err != nil {
		nativeaccess.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": graph})
}

func (s *Service) read(ctx context.Context, r *http.Request) (Graph, error) {
	empty := Graph{Nodes: []Node{}, Edges: []Edge{}}
	// A Key's synthetic owner must never acquire this human-only UI permission.
	if r.Header.Get("X-API-Key") != "" {
		return empty, fail(403, "auth.human_required")
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return empty, fail(401, "auth.required")
	}
	p, err := s.Upstream.CurrentPrincipal(ctx, r.Header)
	if err != nil {
		return empty, err
	}
	if p.User == nil || p.User.ID == "" || !p.User.IsActive || p.Tenant == nil || p.Tenant.ID == 0 {
		return empty, fail(403, "graph.access_denied")
	}
	if requested := r.Header.Get("X-Tenant-ID"); requested != "" && requested != strconv.FormatUint(p.Tenant.ID, 10) {
		return empty, fail(403, "workspace.denied")
	}
	allowed := false
	for _, m := range p.Memberships {
		if m.TenantID == p.Tenant.ID && (m.Role == "owner" || m.Role == "admin" || m.Role == "contributor") {
			allowed = true
		}
	}
	if !allowed {
		return empty, fail(403, "graph.role_denied")
	}
	q := Query{KBID: r.PathValue("kb_id"), Search: strings.TrimSpace(r.URL.Query().Get("q")), Relation: r.URL.Query().Get("relation"), NodeID: r.URL.Query().Get("node_id"), Limit: 80}
	for k, values := range r.URL.Query() {
		if len(values) != 1 || (k != "q" && k != "relation" && k != "node_id" && k != "limit") {
			return empty, fail(400, "graph.invalid_query")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		q.Limit, err = strconv.Atoi(v)
		if err != nil {
			return empty, fail(400, "graph.invalid_query")
		}
	}
	if !identifier.MatchString(q.KBID) || len([]rune(q.Search)) > 100 || len(q.Relation) > 256 || len(q.NodeID) > 256 || q.Limit < 1 || q.Limit > 160 {
		return empty, fail(400, "graph.invalid_query")
	}
	// Resolve the KB using the original human credential on every request.
	if s.KnowledgeAccess != nil {
		if err := s.KnowledgeAccess(ctx, q.KBID, r.Header); err != nil {
			return empty, err
		}
	}
	raw, err := s.Upstream.NativeRequest(ctx, "GET", "/api/v1/knowledge-bases/"+url.PathEscape(q.KBID), nil, r.Header, nil)
	if err != nil {
		return empty, err
	}
	var kb struct {
		Success bool `json:"success"`
		Data    struct {
			ID       string `json:"id"`
			TenantID uint64 `json:"tenant_id"`
			Index    struct {
				Graph bool `json:"graph_enabled"`
			} `json:"indexing_strategy"`
			Extract struct {
				Enabled bool `json:"enabled"`
			} `json:"extract_config"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &kb) != nil || !kb.Success || kb.Data.ID != q.KBID {
		return empty, fail(502, "upstream.invalid_response")
	}
	if kb.Data.TenantID != p.Tenant.ID {
		return empty, fail(403, "workspace.denied")
	}
	if !s.Enabled || s.Reader == nil {
		return empty, fail(409, "graph.dependencies_unavailable")
	}
	if !kb.Data.Index.Graph && !kb.Data.Extract.Enabled {
		empty.State = "disabled"
		return empty, nil
	}
	graph, err := s.Reader.Read(ctx, q)
	if err != nil {
		return empty, fail(503, "graph.unavailable")
	}
	// Deleted or moved documents can outlive asynchronous Neo4j cleanup.
	// Suppress their nodes/edges and verify source visibility with native APIs.
	docs, checked, nodes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, n := range graph.Nodes {
		if !checked[n.KnowledgeID] {
			checked[n.KnowledgeID] = true
			if identifier.MatchString(n.KnowledgeID) {
				parent, e := s.Upstream.KnowledgeBaseForKnowledge(ctx, n.KnowledgeID, r.Header)
				if e != nil {
					var upstream *weknora.Error
					if !errors.As(e, &upstream) || (upstream.StatusCode != 403 && upstream.StatusCode != 404) {
						return empty, fail(502, "graph.source_unavailable")
					}
				}
				docs[n.KnowledgeID] = e == nil && parent == q.KBID
			}
		}
		if docs[n.KnowledgeID] {
			empty.Nodes = append(empty.Nodes, n)
			nodes[n.ID] = true
		}
	}
	for _, e := range graph.Edges {
		if nodes[e.Source] && nodes[e.Target] {
			empty.Edges = append(empty.Edges, e)
		}
	}
	empty.Truncated = graph.Truncated
	empty.State = "ready"
	if len(empty.Nodes) == 0 {
		empty.State = "empty"
	}
	return empty, nil
}
