package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	sessionhandler "github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/gin-gonic/gin"
)

type mindCreekRouteRule struct {
	ID             string `json:"id"`
	Classification string `json:"classification"`
	PathRegex      string `json:"path_regex"`
	compiled       *regexp.Regexp
}

type mindCreekRoutePolicy struct {
	SchemaVersion      int                  `json:"schema_version"`
	UpstreamTag        string               `json:"upstream_tag"`
	UpstreamCommit     string               `json:"upstream_commit"`
	ExpectedRouteCount int                  `json:"expected_route_count"`
	Rules              []mindCreekRouteRule `json:"rules"`
}

func TestMindCreekPhase1RoutePolicyCoverage(t *testing.T) {
	policy := loadMindCreekRoutePolicy(t)
	routes := mindCreekUpstreamRoutes(t)
	if len(routes) != policy.ExpectedRouteCount {
		t.Fatalf("route count = %d, want %d; review the pinned route surface and policy", len(routes), policy.ExpectedRouteCount)
	}

	counts := map[string]int{}
	seen := map[string]struct{}{}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate route in inventory: %s", key)
		}
		seen[key] = struct{}{}

		classification, _ := classifyMindCreekRoute(policy.Rules, route.Path)
		if classification == "" {
			t.Errorf("unclassified route: %s", key)
			continue
		}
		counts[classification]++
	}

	for _, required := range []string{"disabled", "kb_policy_controlled", "pass_through"} {
		if counts[required] == 0 {
			t.Errorf("classification %q has no routes", required)
		}
	}
	for _, required := range mindCreekPersonalNotesEnforcementSamples() {
		classification, _ := classifyMindCreekRoute(policy.Rules, required)
		if classification != "kb_policy_controlled" {
			t.Errorf("Personal Notes enforcement point %s classified as %q", required, classification)
		}
	}

	fmt.Printf("MindCreek Phase 1 route policy verified: %d routes; disabled=%d, kb_policy_controlled=%d, pass_through=%d\n",
		len(routes), counts["disabled"], counts["kb_policy_controlled"], counts["pass_through"])
}

func loadMindCreekRoutePolicy(t *testing.T) mindCreekRoutePolicy {
	t.Helper()
	path := os.Getenv("MINDCREEK_ROUTE_POLICY")
	if path == "" {
		t.Fatal("MINDCREEK_ROUTE_POLICY is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read route policy: %v", err)
	}
	var policy mindCreekRoutePolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("parse route policy: %v", err)
	}
	if policy.SchemaVersion != 1 || policy.UpstreamTag != "v0.7.2" || policy.UpstreamCommit != "3d5d8bfcdfeeea266b292b71cea616847af28d0f" {
		t.Fatalf("route policy targets an unexpected upstream baseline: %#v", policy)
	}
	validClassifications := map[string]bool{"disabled": true, "kb_policy_controlled": true, "pass_through": true}
	for i := range policy.Rules {
		rule := &policy.Rules[i]
		if rule.ID == "" || !validClassifications[rule.Classification] {
			t.Fatalf("invalid route rule: %#v", rule)
		}
		rule.compiled, err = regexp.Compile(rule.PathRegex)
		if err != nil {
			t.Fatalf("compile route rule %s: %v", rule.ID, err)
		}
	}
	return policy
}

func classifyMindCreekRoute(rules []mindCreekRouteRule, path string) (string, string) {
	for _, rule := range rules {
		if rule.compiled.MatchString(path) {
			return rule.Classification, rule.ID
		}
	}
	return "", ""
}

func mindCreekPersonalNotesEnforcementSamples() []string {
	return []string{
		"/api/v1/knowledge-bases",
		"/api/v1/knowledge-bases/:id/knowledge/manual",
		"/api/v1/knowledge/:id/preview",
		"/api/v1/chunks/by-id/:id",
		"/api/v1/knowledge-search",
		"/api/v1/knowledge-chat/:session_id",
		"/api/v1/agent-chat/:session_id",
		"/api/v1/sessions",
		"/api/v1/messages/:session_id/load",
		"/api/v1/knowledgebase/:kb_id/wiki/pages",
		"/api/v1/knowledge-bases/:id/files",
		"/api/v1/knowledge-bases/:id/shares",
		"/api/v1/shared-knowledge-bases",
	}
}

func mindCreekUpstreamRoutes(t *testing.T) []gin.RouteInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("LOCAL_STORAGE_BASE_DIR", t.TempDir())
	r := gin.New()
	rbacEnabled := true
	routerConfig := &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &rbacEnabled}}
	r.GET("/health", func(*gin.Context) {})
	r.GET("/swagger/*any", func(*gin.Context) {})
	RegisterIMRoutes(r, &handler.IMHandler{})
	serveFilesWithResources(r, nil, nil, nil)
	servePresignedFiles(r, nil, nil)
	servePresignedPreview(r, routerConfig, nil)

	v1 := r.Group("/api/v1")
	g := newRBACGuards(routerConfig, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	RegisterAuthRoutes(v1, &handler.AuthHandler{}, g)
	RegisterTenantRoutes(v1, &handler.TenantHandler{}, &handler.TenantMemberHandler{}, &handler.TenantInvitationHandler{}, &handler.AuditLogHandler{}, g)
	RegisterMyInvitationRoutes(v1, &handler.TenantInvitationHandler{})
	RegisterKnowledgeBaseRoutes(v1, &handler.KnowledgeBaseHandler{}, g)
	RegisterKnowledgeBaseActivityRoutes(v1, &handler.AuditLogHandler{}, g)
	serveKBScopedFiles(v1, g, nil, nil, nil, nil)
	RegisterKnowledgeTagRoutes(v1, &handler.TagHandler{}, g)
	RegisterKnowledgeRoutes(v1, &handler.KnowledgeHandler{}, g)
	RegisterFAQRoutes(v1, &handler.FAQHandler{}, g)
	RegisterChunkRoutes(v1, &handler.ChunkHandler{}, g)
	RegisterSessionRoutes(v1, &sessionhandler.Handler{}, &handler.MessageSuggestionHandler{}, g)
	RegisterChatRoutes(v1, &sessionhandler.Handler{}, g)
	RegisterMessageRoutes(v1, &handler.MessageHandler{}, g)
	RegisterModelRoutes(v1, &handler.ModelHandler{}, &handler.ModelCredentialsHandler{}, g)
	RegisterEvaluationRoutes(v1, &handler.EvaluationHandler{}, g)
	RegisterInitializationRoutes(v1, &handler.InitializationHandler{}, g)
	RegisterSystemRoutes(v1, &handler.SystemHandler{}, g)
	RegisterSystemAdminRoutes(v1, &handler.SystemHandler{}, &handler.AuditLogHandler{}, g)
	RegisterMCPServiceRoutes(v1, &handler.MCPServiceHandler{}, &handler.MCPCredentialsHandler{}, &handler.MCPOAuthHandler{}, g)
	RegisterWebSearchRoutes(v1, &handler.WebSearchHandler{}, g)
	RegisterWebSearchProviderRoutes(v1, &handler.WebSearchProviderHandler{}, &handler.WebSearchProviderCredentialsHandler{}, g)
	RegisterVectorStoreRoutes(v1, &handler.VectorStoreHandler{}, g)
	RegisterStorageBackendRoutes(v1, &handler.StorageBackendHandler{}, g)
	RegisterCustomAgentRoutes(v1, &handler.CustomAgentHandler{}, g)
	RegisterUserFavoriteRoutes(v1, &handler.UserResourceFavoriteHandler{}, g)
	RegisterSkillRoutes(v1, &handler.SkillHandler{}, g)
	RegisterOrganizationRoutes(v1, &handler.OrganizationHandler{}, g)
	RegisterIMChannelRoutes(v1, &handler.IMHandler{}, g)
	RegisterEmbedChannelRoutes(v1, &handler.EmbedChannelHandler{}, g)
	RegisterDataSourceRoutes(v1, &handler.DataSourceHandler{}, &handler.DataSourceCredentialsHandler{}, g)
	RegisterWeKnoraCloudRoutes(v1, &handler.WeKnoraCloudHandler{}, g)
	RegisterWikiPageRoutes(v1, &handler.WikiPageHandler{}, g)
	RegisterChunkerDebugRoutes(v1, g)

	routes := append(r.Routes(), mindCreekConditionalRoutes()...)
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func mindCreekConditionalRoutes() []gin.RouteInfo {
	return []gin.RouteInfo{
		{Method: http.MethodGet, Path: "/r/:token"},
		{Method: http.MethodHead, Path: "/r/:token"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/exchange"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/config"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/suggested-questions"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/chunks/:chunk_id"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/knowledge-chat/:session_id"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/agent-chat/:session_id"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/messages/:session_id/load"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/stop"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/sessions/:session_id/messages/:message_id/suggestions"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/messages/:message_id/suggestions"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/suggestion-events"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/events"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/mcp-oauth-resolutions/:pending_id"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/mcp-oauth-resolutions/:pending_id/cancel"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/mcp-services/:id/oauth/authorize-url"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/sessions/:session_id/mcp-services/:id/oauth/status"},
		{Method: http.MethodPost, Path: "/api/v1/embed/:channel_id/sessions/:session_id/tool-approvals/:pending_id"},
		{Method: http.MethodGet, Path: "/api/v1/embed/:channel_id/files"},
	}
}
