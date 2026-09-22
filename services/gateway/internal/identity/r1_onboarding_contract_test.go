package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// R1 evidence of the existing gap, not the intended R2 onboarding behavior.
func TestR1CorporateGateRequiresWorkspaceBeforeBinding(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	account, err := store.Upsert(ctx, Claims{Issuer: "https://r1.example.invalid", Subject: "employee", CorporateEmail: "employee@example.invalid"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	gate, err := NewGate(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = gate.Check(ctx, weknora.Principal{User: &weknora.User{ID: "r1-employee", Email: account.UpstreamEmail}})
	if !errors.Is(err, ErrUnlinked) || store.identity.LocalUserID != "" {
		t.Fatalf("expected tenantless denial without binding, error=%v", err)
	}
}
