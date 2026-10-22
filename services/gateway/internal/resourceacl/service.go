package resourceacl

import (
	"context"
	"errors"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
)

type Catalog interface {
	Resolve(context.Context, uint64, string) (Resource, error)
	MessageImage(context.Context, uint64, string, string, string) (bool, error)
}

type Access interface {
	Authorize(context.Context, http.Header, string, ...string) (kbacl.Permissions, error)
	ResourceIdentity(context.Context, http.Header) (uint64, bool, error)
}

type Service struct {
	Catalog Catalog
	Access  Access
}

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string      { return e.Code }
func (e *Error) HTTPStatus() int    { return e.Status }
func (e *Error) PublicCode() string { return e.Code }
func fail(status int, code string) error {
	return &Error{Status: status, Code: "kb_acl." + code}
}

func (s *Service) resolve(ctx context.Context, h http.Header, ref string) (Resource, bool, error) {
	if s.Catalog == nil || s.Access == nil {
		return Resource{}, false, fail(503, "resource_verification_unavailable")
	}
	tenant, manager, err := s.Access.ResourceIdentity(ctx, h)
	if err != nil {
		return Resource{}, false, err
	}
	resource, err := s.Catalog.Resolve(ctx, tenant, ref)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidReference) {
			return Resource{}, false, fail(404, "not_found")
		}
		return Resource{}, false, fail(503, "resource_verification_unavailable")
	}
	if resource.TenantID != tenant {
		return Resource{}, false, fail(404, "not_found")
	}
	return resource, manager, nil
}

// AuthorizeKB never treats editable content or the URL's KB identifier as
// evidence of ownership. The native file handler still verifies tenant/storage.
func (s *Service) AuthorizeKB(ctx context.Context, h http.Header, kb, ref string) error {
	if s.Access == nil {
		return fail(503, "resource_verification_unavailable")
	}
	if _, err := s.Access.Authorize(ctx, h, kb); err != nil {
		return err
	}
	resource, manager, err := s.resolve(ctx, h, ref)
	if err != nil {
		return err
	}
	if manager && (!resource.SessionScoped || len(resource.Owners) > 0) {
		return nil
	}
	if resource.SessionScoped && len(resource.Owners) == 0 {
		return fail(404, "not_found")
	}
	for _, owner := range resource.Owners {
		if owner.KBID == kb {
			return nil
		}
	}
	if len(resource.Owners) == 0 {
		return fail(409, "resource_binding_unavailable")
	}
	return fail(404, "not_found")
}

// AuthorizeMessage is called only after fresh session owner/isolation checks.
// KB images must still be in that session's scope and currently visible.
// Native user-image attachments have a separate immutable upload provenance.
func (s *Service) AuthorizeMessage(ctx context.Context, h http.Header, session, message string, kbs []string, ref string) error {
	resource, manager, err := s.resolve(ctx, h, ref)
	if err != nil {
		return err
	}
	uploaded, err := s.Catalog.MessageImage(ctx, resource.TenantID, session, message, ref)
	if err != nil {
		return fail(503, "resource_verification_unavailable")
	}
	if uploaded {
		return nil
	}
	if resource.SessionScoped && len(resource.Owners) == 0 {
		return fail(404, "not_found")
	}
	if manager {
		return nil
	}
	for _, owner := range resource.Owners {
		for _, kb := range kbs {
			if owner.KBID == kb {
				_, err := s.Access.Authorize(ctx, h, kb)
				return err
			}
		}
	}
	if len(resource.Owners) == 0 {
		return fail(409, "resource_binding_unavailable")
	}
	return fail(404, "not_found")
}
