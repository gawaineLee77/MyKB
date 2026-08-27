// Package routeaction loads the verified Phase 2 method/path action inventory.
package routeaction

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
)

const supportedCommit = "3d5d8bfcdfeeea266b292b71cea616847af28d0f"

type document struct {
	SchemaVersion        int    `json:"schema_version"`
	UpstreamTag          string `json:"upstream_tag"`
	UpstreamCommit       string `json:"upstream_commit"`
	ExpectedKBRouteCount int    `json:"expected_kb_route_count"`
	Rules                []rule `json:"rules"`
}

type rule struct {
	ID          string               `json:"id"`
	Action      authorization.Action `json:"action"`
	MethodRegex string               `json:"method_regex"`
	PathRegex   string               `json:"path_regex"`
	method      *regexp.Regexp
	path        *regexp.Regexp
}

type Policy struct {
	rules []rule
}

func Load(filename, upstreamVersion string) (*Policy, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open route-action policy: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 256<<10))
	decoder.DisallowUnknownFields()
	var value document
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode route-action policy: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("route-action policy must contain one JSON document")
	}
	if value.SchemaVersion != 1 || value.UpstreamTag != upstreamVersion ||
		value.UpstreamCommit != supportedCommit || value.ExpectedKBRouteCount != 166 {
		return nil, fmt.Errorf("route-action policy does not match the verified WeKnora v0.7.2 contract")
	}
	if len(value.Rules) == 0 {
		return nil, fmt.Errorf("route-action policy has no rules")
	}
	ids := make(map[string]bool, len(value.Rules))
	for index := range value.Rules {
		item := &value.Rules[index]
		if item.ID == "" || ids[item.ID] || !item.Action.Valid() {
			return nil, fmt.Errorf("route-action rule %q is invalid", item.ID)
		}
		ids[item.ID] = true
		item.method, err = regexp.Compile(item.MethodRegex)
		if err != nil {
			return nil, fmt.Errorf("compile method rule %q: %w", item.ID, err)
		}
		item.path, err = regexp.Compile(expandGinParameters(item.PathRegex))
		if err != nil {
			return nil, fmt.Errorf("compile path rule %q: %w", item.ID, err)
		}
	}
	return &Policy{rules: value.Rules}, nil
}

// Match returns an action only when exactly one rule matches.
func (p *Policy) Match(method, requestPath string) (authorization.Action, bool) {
	var result authorization.Action
	matches := 0
	for _, item := range p.rules {
		if item.method.MatchString(method) && item.path.MatchString(requestPath) {
			result = item.Action
			matches++
		}
	}
	return result, matches == 1
}

var (
	ginParameter = regexp.MustCompile(`/:[A-Za-z_][A-Za-z0-9_]*`)
	ginWildcard  = regexp.MustCompile(`/\*[A-Za-z_][A-Za-z0-9_]*`)
)

func expandGinParameters(pattern string) string {
	pattern = ginParameter.ReplaceAllString(pattern, `/[^/]+`)
	return ginWildcard.ReplaceAllString(pattern, `/.*`)
}
