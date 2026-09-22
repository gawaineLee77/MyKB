// Package nativeaccess adds request scope and session isolation to native
// WeKnora authorization. It never derives a write grant from a successful read.
package nativeaccess

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type Route struct {
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	Disposition  string   `json:"disposition"`
	Checks       []string `json:"checks"`
	BaselineRule string   `json:"baseline_rule"`
	Handler      string   `json:"handler"`
	pattern      *regexp.Regexp
}

type Routes struct{ items []Route }

func LoadRoutes(filename string) (*Routes, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var document struct {
		SchemaVersion int     `json:"schema_version"`
		Tag           string  `json:"upstream_tag"`
		Commit        string  `json:"upstream_commit"`
		Routes        []Route `json:"routes"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if document.SchemaVersion != 1 || document.Tag != "v0.8.0" || document.Commit != "1edcd54b43606d9079bb36650efe3f68707a79ea" || len(document.Routes) != 429 {
		return nil, fmt.Errorf("native route manifest does not match the pinned upstream")
	}
	seen := map[string]bool{}
	for i := range document.Routes {
		r := &document.Routes[i]
		key := r.Method + " " + r.Path
		if seen[key] || !strings.HasPrefix(r.Path, "/") || r.Method == "" || (r.Disposition != "native" && r.Disposition != "disabled") {
			return nil, fmt.Errorf("invalid native route %q", key)
		}
		seen[key] = true
		parts := strings.Split(r.Path, "/")
		for j, part := range parts {
			switch {
			case strings.HasPrefix(part, ":"):
				parts[j] = "[^/]+"
			case strings.HasPrefix(part, "*"):
				parts[j] = ".*"
			default:
				parts[j] = regexp.QuoteMeta(part)
			}
		}
		r.pattern, err = regexp.Compile("^" + strings.Join(parts, "/") + "$")
		if err != nil {
			return nil, err
		}
	}
	// Static routes win over parameters, matching Gin's route dispatch.
	sort.SliceStable(document.Routes, func(i, j int) bool {
		return specificity(document.Routes[i].Path) > specificity(document.Routes[j].Path)
	})
	return &Routes{items: document.Routes}, nil
}

func specificity(path string) int {
	score := 0
	for _, part := range strings.Split(path, "/") {
		if part != "" && !strings.HasPrefix(part, ":") && !strings.HasPrefix(part, "*") {
			score += 100 + len(part)
		}
	}
	return score
}

func (r *Routes) Match(method, path string) (Route, bool) {
	for _, item := range r.items {
		if method == item.Method && item.pattern.MatchString(path) {
			return item, true
		}
	}
	return Route{}, false
}

var retired = []*regexp.Regexp{
	regexp.MustCompile(`^/api/v1/knowledge-spaces(?:/|$)`),
	regexp.MustCompile(`^/api/v1/knowledge-bases/[^/]+/(?:product-profile|notes|ingestions)(?:/|$)`),
	regexp.MustCompile(`^/api/v1/mindcreek/(?:knowledge-bases|users|catalog|publications|me/subscriptions)(?:/|$)`),
}

// Retired includes only the old product namespaces. Native manual documents,
// FAQ, Wiki and sharing routes remain part of the pinned route manifest.
func Retired(path string) bool {
	for _, pattern := range retired {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}
