package kbacl

import (
	"context"
	"net/http"
)

// ResourceIdentity distinguishes a real current workspace manager from a KB
// creator, an installation administrator in another workspace, and an API key
// whose native auth/me response projects a human owner.
func (s *Service) ResourceIdentity(ctx context.Context, h http.Header) (uint64, bool, error) {
	p, err := s.principal(ctx, h, false)
	if err != nil {
		return 0, false, err
	}
	return p.Tenant.ID, h.Get("X-API-Key") == "" && isManager(nativeRole(p)), nil
}
