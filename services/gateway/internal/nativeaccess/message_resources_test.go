package nativeaccess

import (
	"context"
	"net/http"
	"testing"
)

func TestMessageResourceCheckFollowsSessionIsolation(t *testing.T) {
	s, _, store, actor := testScope()
	ctx := context.Background()
	visible := true
	s.KnowledgeAccess = func(context.Context, string, http.Header) error {
		if !visible {
			return &Error{404, "kb_acl.not_found"}
		}
		return nil
	}
	calls := 0
	s.MessageResource = func(_ context.Context, _ http.Header, session, message string, kbs []string, ref string) error {
		calls++
		if session != "session" || message != "message" || ref != "resource://synthetic" || len(kbs) != 1 || kbs[0] != "a" {
			t.Fatal(session, message, kbs, ref)
		}
		return nil
	}
	check := func(wantError bool) {
		t.Helper()
		r := requestJSON("GET", "/api/v1/sessions/session/messages/message/files?file_path=resource://synthetic", "")
		err := s.Check(ctx, r, actor)
		if (err != nil) != wantError {
			t.Fatal(err)
		}
	}
	check(true)
	if calls != 0 {
		t.Fatal("unbound session reached resource verifier")
	}
	if err := store.Bind(ctx, "session", actor, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	check(false)
	visible = false
	check(true)
	if calls != 1 {
		t.Fatal("revoked session reached resource verifier")
	}
}
