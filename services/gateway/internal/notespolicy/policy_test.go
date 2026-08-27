package notespolicy

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

func TestPersonalNotesOwnerOnlyMatrix(t *testing.T) {
	note := profile.Profile{
		UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice",
		ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly,
	}
	for _, tc := range []struct {
		name      string
		principal Principal
		operation Operation
		wantCode  string
		wantHTTP  int
	}{
		{name: "owner read", principal: Principal{UserID: "alice", TenantID: 42}, operation: Read},
		{name: "owner write", principal: Principal{UserID: "alice", TenantID: 42}, operation: Write},
		{name: "other user", principal: Principal{UserID: "bob", TenantID: 42}, operation: Read, wantCode: "resource.not_found", wantHTTP: http.StatusNotFound},
		{name: "wrong workspace", principal: Principal{UserID: "alice", TenantID: 99}, operation: Read, wantCode: "resource.not_found", wantHTTP: http.StatusNotFound},
		{name: "owner share", principal: Principal{UserID: "alice", TenantID: 42}, operation: Share, wantCode: "personal_notes.sharing_disabled", wantHTTP: http.StatusForbidden},
		{name: "owner publish", principal: Principal{UserID: "alice", TenantID: 42}, operation: Publish, wantCode: "personal_notes.sharing_disabled", wantHTTP: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(note, tc.principal, tc.operation)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("Authorize() error = %v", err)
				}
				return
			}
			var policyError *Error
			if !errors.As(err, &policyError) || policyError.Code != tc.wantCode || policyError.StatusCode != tc.wantHTTP {
				t.Fatalf("Authorize() error = %+v", err)
			}
		})
	}
}

func TestNonNoteProfileUsesUpstreamPolicy(t *testing.T) {
	rag := profile.Profile{ProductMode: profile.ModeRAG, AccessPolicy: profile.PolicyUpstream}
	if err := Authorize(rag, Principal{UserID: "any", TenantID: 1}, Share); err != nil {
		t.Fatalf("Authorize() unexpectedly restricted a RAG profile: %v", err)
	}
}
