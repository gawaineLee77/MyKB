package router

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"testing"
)

type mindCreekPhase2ActionRule struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	MethodRegex string `json:"method_regex"`
	PathRegex   string `json:"path_regex"`
	method      *regexp.Regexp
	path        *regexp.Regexp
}

type mindCreekPhase2ActionPolicy struct {
	SchemaVersion        int                         `json:"schema_version"`
	UpstreamTag          string                      `json:"upstream_tag"`
	UpstreamCommit       string                      `json:"upstream_commit"`
	ExpectedKBRouteCount int                         `json:"expected_kb_route_count"`
	Rules                []mindCreekPhase2ActionRule `json:"rules"`
}

func TestMindCreekPhase2RouteActionCoverage(t *testing.T) {
	phase1 := loadMindCreekRoutePolicy(t)
	phase2 := loadMindCreekPhase2ActionPolicy(t)
	routes := mindCreekUpstreamRoutes(t)

	counts := map[string]int{}
	seen := map[string]struct{}{}
	kbRouteCount := 0
	for _, route := range routes {
		classification, _ := classifyMindCreekRoute(phase1.Rules, route.Path)
		if classification != "kb_policy_controlled" {
			continue
		}
		kbRouteCount++
		key := route.Method + " " + route.Path
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate KB route in inventory: %s", key)
		}
		seen[key] = struct{}{}

		action, ruleID, matches := classifyMindCreekPhase2ActionMatches(phase2.Rules, route.Method, route.Path)
		if action == "" {
			t.Errorf("unclassified Phase 2 KB route: %s", key)
			continue
		}
		if matches != 1 {
			t.Errorf("ambiguous Phase 2 KB route: %s matched %d rules; first=%s", key, matches, ruleID)
		}
		counts[action]++
	}

	if kbRouteCount != phase2.ExpectedKBRouteCount {
		t.Fatalf("KB-policy route count = %d, want %d", kbRouteCount, phase2.ExpectedKBRouteCount)
	}
	for _, required := range []string{"discover", "read", "edit_content", "configure", "manage_grants", "delete"} {
		if counts[required] == 0 {
			t.Errorf("Phase 2 action %q has no routes", required)
		}
	}
	assertMindCreekPhase2Samples(t, phase2.Rules)

	fmt.Printf("MindCreek Phase 2 route actions verified: %d routes; discover=%d, read=%d, edit_content=%d, configure=%d, manage_grants=%d, delete=%d\n",
		kbRouteCount, counts["discover"], counts["read"], counts["edit_content"], counts["configure"], counts["manage_grants"], counts["delete"])
}

func TestMindCreekPhase2DumpKBRoutes(t *testing.T) {
	if os.Getenv("MINDCREEK_PHASE2_DUMP") != "1" {
		t.Skip("set MINDCREEK_PHASE2_DUMP=1 to print the pinned KB-policy route surface")
	}
	phase1 := loadMindCreekRoutePolicy(t)
	routes := mindCreekUpstreamRoutes(t)
	result := make([]string, 0)
	for _, route := range routes {
		classification, _ := classifyMindCreekRoute(phase1.Rules, route.Path)
		if classification == "kb_policy_controlled" {
			result = append(result, route.Method+" "+route.Path)
		}
	}
	sort.Strings(result)
	for _, item := range result {
		fmt.Println(item)
	}
}

func loadMindCreekPhase2ActionPolicy(t *testing.T) mindCreekPhase2ActionPolicy {
	t.Helper()
	path := os.Getenv("MINDCREEK_PHASE2_ROUTE_ACTIONS")
	if path == "" {
		t.Fatal("MINDCREEK_PHASE2_ROUTE_ACTIONS is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Phase 2 route actions: %v", err)
	}
	var policy mindCreekPhase2ActionPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("parse Phase 2 route actions: %v", err)
	}
	if policy.SchemaVersion != 1 || policy.UpstreamTag != "v0.7.2" || policy.UpstreamCommit != "3d5d8bfcdfeeea266b292b71cea616847af28d0f" {
		t.Fatalf("Phase 2 route actions target an unexpected upstream baseline: %#v", policy)
	}
	validActions := map[string]bool{
		"discover": true, "read": true, "edit_content": true,
		"configure": true, "manage_grants": true, "delete": true,
	}
	for index := range policy.Rules {
		rule := &policy.Rules[index]
		if rule.ID == "" || !validActions[rule.Action] {
			t.Fatalf("invalid Phase 2 action rule: %#v", rule)
		}
		rule.method, err = regexp.Compile(rule.MethodRegex)
		if err != nil {
			t.Fatalf("compile method rule %s: %v", rule.ID, err)
		}
		rule.path, err = regexp.Compile(rule.PathRegex)
		if err != nil {
			t.Fatalf("compile path rule %s: %v", rule.ID, err)
		}
	}
	return policy
}

func classifyMindCreekPhase2Action(rules []mindCreekPhase2ActionRule, method, path string) (string, string) {
	action, ruleID, _ := classifyMindCreekPhase2ActionMatches(rules, method, path)
	return action, ruleID
}

func classifyMindCreekPhase2ActionMatches(rules []mindCreekPhase2ActionRule, method, path string) (string, string, int) {
	var action string
	var ruleID string
	matches := 0
	for _, rule := range rules {
		if rule.method.MatchString(method) && rule.path.MatchString(path) {
			matches++
			if action == "" {
				action = rule.Action
				ruleID = rule.ID
			}
		}
	}
	return action, ruleID, matches
}

func assertMindCreekPhase2Samples(t *testing.T, rules []mindCreekPhase2ActionRule) {
	t.Helper()
	samples := []struct {
		method string
		path   string
		action string
	}{
		{"GET", "/api/v1/knowledge-bases", "discover"},
		{"POST", "/api/v1/knowledge-search", "read"},
		{"POST", "/api/v1/knowledge-chat/:session_id", "read"},
		{"POST", "/api/v1/sessions", "read"},
		{"PUT", "/api/v1/knowledge-bases/:id/pin", "read"},
		{"POST", "/api/v1/knowledge-bases/:id/knowledge/manual", "edit_content"},
		{"DELETE", "/api/v1/knowledge/:id", "edit_content"},
		{"PUT", "/api/v1/knowledge-bases/:id", "configure"},
		{"GET", "/api/v1/knowledge-bases/:id/shares", "manage_grants"},
		{"POST", "/api/v1/knowledge-bases/:id/shares", "manage_grants"},
		{"DELETE", "/api/v1/knowledge-bases/:id", "delete"},
	}
	for _, sample := range samples {
		action, ruleID := classifyMindCreekPhase2Action(rules, sample.method, sample.path)
		if action != sample.action {
			t.Errorf("%s %s classified as %q by %q, want %q", sample.method, sample.path, action, ruleID, sample.action)
		}
	}
}
