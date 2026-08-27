// Package notespolicy defines the non-bypassable Personal Notes rules.
package notespolicy

import (
	"fmt"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

type Operation string

const (
	Read    Operation = "read"
	Write   Operation = "write"
	Share   Operation = "share"
	Publish Operation = "publish"
)

type Principal struct {
	UserID   string
	TenantID uint64
}

type Error struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *Error) Error() string { return e.Code }

// Authorize enforces owner-only Personal Notes without an administrator bypass.
func Authorize(kbProfile profile.Profile, principal Principal, operation Operation) error {
	if kbProfile.ProductMode != profile.ModePersonalNotes {
		return nil
	}
	if kbProfile.AccessPolicy != profile.PolicyOwnerOnly {
		return &Error{Code: "security.profile_invalid", Message: "Personal Notes security profile is invalid", StatusCode: http.StatusServiceUnavailable}
	}
	if principal.UserID != kbProfile.OwnerUserID || principal.TenantID != kbProfile.TenantID {
		// Use the same response as a missing resource to avoid confirming that
		// another user's Note Space exists.
		return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
	}
	switch operation {
	case Read, Write:
		return nil
	case Share, Publish:
		return &Error{Code: "personal_notes.sharing_disabled", Message: "Personal Notes cannot be shared or published", StatusCode: http.StatusForbidden}
	default:
		return fmt.Errorf("unknown Personal Notes operation %q", operation)
	}
}
