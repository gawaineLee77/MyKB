package nativeaccess

import (
	"context"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"net/http"
	"strings"
	"testing"
)

type avatarNativeFixture struct{ *nativeFixture }

func (f avatarNativeFixture) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "employee", IsActive: true}, Tenant: &weknora.Tenant{ID: 1}}, nil
}

func TestAgentAvatarMutationsValidateReferenceAndCopySource(t *testing.T) {
	s, f, _, actor := testScope()
	s.Upstream = avatarNativeFixture{f}
	ctx := context.Background()
	ref := "mc-avatar:" + strings.Repeat("a", 32)
	f.agent["id"] = "agent"
	f.agent["avatar"] = ref
	var gotAgent, gotAvatar string
	s.AgentAvatarCheck = func(_ context.Context, _ http.Header, id, avatar string) error {
		gotAgent = id
		gotAvatar = avatar
		return nil
	}
	for _, tc := range []struct{ method, path, body, wantAgent string }{
		{"POST", "/api/v1/agents", `{"avatar":"` + ref + `"}`, ""},
		{"PUT", "/api/v1/agents/agent", `{"avatar":"` + ref + `"}`, "agent"},
		{"POST", "/api/v1/agents/agent/copy", `{"avatar":"ignored"}`, "agent"},
	} {
		r := requestJSON(tc.method, tc.path, tc.body)
		if err := s.Check(ctx, r, actor); err != nil {
			t.Fatal(err)
		}
		if gotAgent != tc.wantAgent || gotAvatar != ref {
			t.Fatalf("validated %q/%q", gotAgent, gotAvatar)
		}
	}
	s.AgentAvatarCheck = func(context.Context, http.Header, string, string) error { return &Error{404, "agent_icon.not_found"} }
	r := requestJSON("POST", "/api/v1/agents", `{"avatar":"`+ref+`"}`)
	if err := s.Check(ctx, r, actor); err == nil {
		t.Fatal("unowned reference admitted")
	}
}

func TestAgentIconReadRechecksKBVisibility(t *testing.T) {
	s, f, _, _ := testScope()
	s.Upstream = avatarNativeFixture{f}
	f.agent["id"] = "agent"
	f.agent["avatar"] = "mc-avatar:" + strings.Repeat("a", 32)
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return &Error{404, "kb_acl.not_found"} }
	if _, err := s.AgentAvatarForRead(context.Background(), "agent", headerForACL()); err == nil {
		t.Fatal("hidden KB metadata icon returned")
	}
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return nil }
	f.agent["tenant_id"] = float64(2)
	if _, err := s.AgentAvatarForRead(context.Background(), "agent", http.Header{"Authorization": {"Bearer synthetic"}}); err == nil {
		t.Fatal("header omission admitted other tenant icon")
	}
}
