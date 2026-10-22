// Package kbacl owns knowledge-base visibility and explicit collaboration grants.
// Native membership remains the authority for workspace roles.
package kbacl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var Operations = []string{"document.create", "document.update", "document.delete", "document.download", "chunk.update", "chunk.delete", "chunk.export"}
var ErrNotFound = errors.New("kb acl record not found")
var ErrConflict = errors.New("kb acl revision conflict")

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string           { return e.Code }
func (e *Error) HTTPStatus() int         { return e.Status }
func (e *Error) PublicCode() string      { return e.Code }
func fail(status int, code string) error { return &Error{Status: status, Code: "kb_acl." + code} }

type Permissions struct {
	Visible    bool     `json:"visible"`
	Manage     bool     `json:"manage"`
	Creator    bool     `json:"creator"`
	Visibility string   `json:"visibility"`
	Revision   uint64   `json:"revision"`
	Operations []string `json:"operations"`
}
type Generation struct {
	Member   uint64
	Identity uint64
}
type auditActorKey struct{}

// WithAuditActor attributes a lifecycle callback to its authenticated human.
// Populate this only after resolving the native principal on the server.
func WithAuditActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, auditActorKey{}, userID)
}
func auditActor(ctx context.Context) string {
	if user, _ := ctx.Value(auditActorKey{}).(string); validID(user) {
		return user
	}
	return "system"
}

type Member struct {
	UserID     string     `json:"user_id"`
	Visible    bool       `json:"visible"`
	Operations []string   `json:"operations"`
	Generation Generation `json:"-"`
}
type Policy struct {
	TenantID          uint64     `json:"-"`
	KBID              string     `json:"kb_id"`
	CreatorUserID     string     `json:"creator_user_id"`
	CreatorRevoked    bool       `json:"creator_revoked"`
	CreatorGeneration Generation `json:"-"`
	Visibility        string     `json:"visibility"`
	Revision          uint64     `json:"revision"`
	Deleted           bool       `json:"-"`
	Members           []Member   `json:"members"`
}
type Event struct {
	ID          uint64          `json:"id"`
	TenantID    uint64          `json:"-"`
	KBID        string          `json:"kb_id"`
	ActorUserID string          `json:"actor_user_id"`
	Action      string          `json:"action"`
	Target      string          `json:"target"`
	Revision    uint64          `json:"revision"`
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
	Outcome     string          `json:"outcome"`
	RequestID   string          `json:"request_id"`
	CreatedAt   time.Time       `json:"created_at"`
}
type Store interface {
	Policy(context.Context, uint64, string) (Policy, error)
	Generation(context.Context, uint64, string) (Generation, error)
	Ensure(context.Context, Policy) (Policy, error)
	Save(context.Context, Policy, uint64, Event) (Policy, error)
	Events(context.Context, uint64, string) ([]Event, error)
	RevokeMember(context.Context, uint64, string, bool) error
	RevokeIdentity(context.Context, string) error
	BeginCreation(context.Context, CreateOperation) (CreateOperation, bool, error)
	Creation(context.Context, uint64, string) (CreateOperation, error)
	CreationByKey(context.Context, uint64, string, string) (CreateOperation, error)
	CompleteCreation(context.Context, uint64, string, string) error
	Creations(context.Context, uint64) ([]CreateOperation, error)
	CreationPending(context.Context, uint64, string, string) (bool, error)
}
type CreateOperation struct {
	ID          string    `json:"id"`
	TenantID    uint64    `json:"-"`
	ActorUserID string    `json:"actor_user_id"`
	KeyHash     string    `json:"-"`
	RequestHash string    `json:"request_hash"`
	State       string    `json:"state"`
	KBID        string    `json:"kb_id,omitempty"`
	RequestID   string    `json:"request_id"`
	CreatedAt   time.Time `json:"created_at"`
	BaselineIDs []string  `json:"-"`
}
type Upstream interface {
	CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error)
	NativeRequest(context.Context, string, string, url.Values, http.Header, any) (json.RawMessage, error)
}
type Service struct {
	Upstream              Upstream
	Store                 Store
	IdentityAllowed       func(context.Context, string) error
	MachinePermissions    func(context.Context, http.Header, string) (Permissions, error)
	Members               func(context.Context, http.Header, uint64) ([]weknora.TenantMember, error)
	WithAuthorizationLock func(context.Context, http.Header, []string, func() error) error
}
