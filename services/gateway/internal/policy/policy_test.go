package policy

import (
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRepositoryPolicyRuntimeDecisions(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	for _, tc := range []struct {
		path string
		want Classification
	}{
		{path: "/api/v1/im/channels", want: Disabled},
		{path: "/api/v1/agents/agent-1/im-channels", want: Disabled},
		{path: "/r/opaque-token", want: Disabled},
		{path: "/api/v1/mcp-services", want: Disabled},
		{path: "/api/v1/knowledge-bases/kb-1", want: KBPolicyControlled},
		{path: "/api/v1/auth/config", want: PassThrough},
		{path: "/api/v1/initialization/asr/check", want: Disabled},
	} {
		decision, ok := policy.Match(tc.path)
		if !ok || decision.Classification != tc.want {
			t.Fatalf("Match(%q) = %+v, %t; want %q", tc.path, decision, ok, tc.want)
		}
	}
	if decision, ok := policy.Match("/api/v1/unclassified"); ok {
		t.Fatalf("unexpected decision for unknown route: %+v", decision)
	}
}

func TestNormalizeRequestPathRejectsBypasses(t *testing.T) {
	for _, rawURL := range []string{
		"http://gateway/api/v1/im%2fchannels",
		"http://gateway/api/v1/im%5cchannels",
		"http://gateway/api/v1//im/channels",
	} {
		request := httptest.NewRequest("GET", rawURL, nil)
		if _, err := NormalizeRequestPath(request); err == nil {
			t.Fatalf("NormalizeRequestPath(%q) accepted ambiguous input", rawURL)
		}
	}
}

func loadRepositoryPolicy(t *testing.T) *Policy {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	filename := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../config/phase1-route-policy.json"))
	policy, err := Load(filename, "v0.7.2")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
