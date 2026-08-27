package routeaction

import (
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
)

func TestVerifiedPolicyClassifiesBehaviorNotMethod(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	filename := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../config/phase2-route-actions.json"))
	policy, err := Load(filename, "v0.7.2")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method string
		path   string
		want   authorization.Action
	}{
		{method: "POST", path: "/api/v1/knowledge-bases/kb-1/hybrid-search", want: authorization.ActionRead},
		{method: "POST", path: "/api/v1/knowledge-bases/kb-1/knowledge/file", want: authorization.ActionEditContent},
		{method: "PUT", path: "/api/v1/knowledge-bases/kb-1", want: authorization.ActionConfigure},
		{method: "GET", path: "/api/v1/knowledge-bases/kb-1/shares", want: authorization.ActionManageGrants},
		{method: "DELETE", path: "/api/v1/knowledge-bases/kb-1", want: authorization.ActionDelete},
	}
	for _, test := range tests {
		got, matched := policy.Match(test.method, test.path)
		if !matched || got != test.want {
			t.Errorf("Match(%s %s) = %q, %t; want %q", test.method, test.path, got, matched, test.want)
		}
	}
}

func TestPolicyRejectsUnclassifiedMethodPath(t *testing.T) {
	policy := &Policy{rules: []rule{{
		ID: "read", Action: authorization.ActionRead,
		method: regexpMustCompile("^GET$"), path: regexpMustCompile("^/known$"),
	}}}
	if _, ok := policy.Match("POST", "/known"); ok {
		t.Fatal("unclassified method/path matched")
	}
}

func regexpMustCompile(value string) *regexp.Regexp {
	return regexp.MustCompile(value)
}
