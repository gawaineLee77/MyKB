package kbacl

import (
	"context"
	"errors"
	"testing"
)

func TestResourceIdentityUsesCurrentWorkspaceMembershipNotCreatorOrKey(t *testing.T) {
	for _, role := range []string{"owner", "admin", "contributor", "viewer"} {
		t.Run(role, func(t *testing.T) {
			s, _, _, h := setup("creator", role)
			tenant, manager, err := s.ResourceIdentity(context.Background(), h)
			if err != nil || tenant != 7 || manager != (role == "owner" || role == "admin") {
				t.Fatal(tenant, manager, err)
			}
			h.Del("Authorization")
			h.Set("X-API-Key", "synthetic")
			_, manager, err = s.ResourceIdentity(context.Background(), h)
			if err != nil || manager {
				t.Fatal("API key acquired human manager bypass", manager, err)
			}
		})
	}
	s, _, _, h := setup("creator", "owner")
	s.IdentityAllowed = func(context.Context, string) error { return errors.New("suspended") }
	if _, _, err := s.ResourceIdentity(context.Background(), h); err == nil {
		t.Fatal("suspended identity accepted")
	}
}
