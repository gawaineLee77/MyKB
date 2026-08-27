// Package policy classifies normalized requests before they reach WeKnora.
package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

const supportedCommit = "3d5d8bfcdfeeea266b292b71cea616847af28d0f"

type Classification string

const (
	Disabled           Classification = "disabled"
	KBPolicyControlled Classification = "kb_policy_controlled"
	PassThrough        Classification = "pass_through"
)

type filePolicy struct {
	SchemaVersion      int    `json:"schema_version"`
	UpstreamTag        string `json:"upstream_tag"`
	UpstreamCommit     string `json:"upstream_commit"`
	ExpectedRouteCount int    `json:"expected_route_count"`
	Rules              []Rule `json:"rules"`
}

type Rule struct {
	ID             string         `json:"id"`
	Classification Classification `json:"classification"`
	PathRegex      string         `json:"path_regex"`
	compiled       *regexp.Regexp
}

type Policy struct {
	rules []Rule
}

type Decision struct {
	Classification Classification
	RuleID         string
}

// Load validates the policy against the exact upstream release.
func Load(filename, upstreamVersion string) (*Policy, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open route policy: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 256<<10))
	decoder.DisallowUnknownFields()
	var document filePolicy
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode route policy: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	if document.SchemaVersion != 1 || document.UpstreamTag != upstreamVersion || document.UpstreamCommit != supportedCommit || document.ExpectedRouteCount != 373 {
		return nil, fmt.Errorf("route policy does not match the verified WeKnora v0.7.2 contract")
	}
	if len(document.Rules) == 0 {
		return nil, fmt.Errorf("route policy has no rules")
	}
	ids := make(map[string]bool, len(document.Rules))
	for index := range document.Rules {
		rule := &document.Rules[index]
		if rule.ID == "" || ids[rule.ID] {
			return nil, fmt.Errorf("route policy has an empty or duplicate rule ID %q", rule.ID)
		}
		ids[rule.ID] = true
		switch rule.Classification {
		case Disabled, KBPolicyControlled, PassThrough:
		default:
			return nil, fmt.Errorf("route rule %q has invalid classification %q", rule.ID, rule.Classification)
		}
		requestPattern := expandGinParameters(rule.PathRegex)
		rule.compiled, err = regexp.Compile(requestPattern)
		if err != nil {
			return nil, fmt.Errorf("compile route rule %q: %w", rule.ID, err)
		}
	}
	return &Policy{rules: document.Rules}, nil
}

// Match applies the ordered, first-match-wins policy.
func (p *Policy) Match(requestPath string) (Decision, bool) {
	for _, rule := range p.rules {
		if rule.compiled.MatchString(requestPath) {
			return Decision{Classification: rule.Classification, RuleID: rule.ID}, true
		}
	}
	return Decision{}, false
}

// NormalizeRequestPath rejects ambiguous path encodings before classification.
func NormalizeRequestPath(request *http.Request) (string, error) {
	escaped := strings.ToLower(request.URL.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%00") {
		return "", fmt.Errorf("encoded separator or NUL")
	}
	requestPath := request.URL.Path
	if requestPath == "" || !strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, "\\") || strings.ContainsRune(requestPath, '\x00') {
		return "", fmt.Errorf("invalid path")
	}
	cleaned := path.Clean(requestPath)
	if requestPath != cleaned && requestPath != cleaned+"/" {
		return "", fmt.Errorf("non-canonical path")
	}
	return requestPath, nil
}

var (
	ginParameter = regexp.MustCompile(`/:[A-Za-z_][A-Za-z0-9_]*`)
	ginWildcard  = regexp.MustCompile(`/\*[A-Za-z_][A-Za-z0-9_]*`)
)

func expandGinParameters(pattern string) string {
	pattern = ginParameter.ReplaceAllString(pattern, `/[^/]+`)
	return ginWildcard.ReplaceAllString(pattern, `/.*`)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("route policy must contain one JSON document")
		}
		return fmt.Errorf("decode route policy suffix: %w", err)
	}
	return nil
}
