// Package agenticon stores bounded, authenticated custom Agent pictures outside
// the pinned native database. Native avatar fields hold only a short reference.
package agenticon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/safeimage"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const Prefix = "mc-avatar:"
const MaxImages = 100
const MaxStoredBytes = 10 << 20

var referenceID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var ErrNotFound = errors.New("agent_icon.not_found")
var ErrQuota = errors.New("agent_icon.quota_exceeded")

type Image struct {
	ID       string
	TenantID uint64
	UserID   string
	PNG      []byte
}
type Store interface {
	Put(context.Context, Image) error
	Get(context.Context, uint64, string) (Image, error)
}
type Service struct {
	Upstream           nativeaccess.PrincipalResolver
	Store              Store
	IdentityAllowed    func(context.Context, string) error
	AgentAvatarForRead func(context.Context, string, http.Header) (string, error)
}

func fail(status int, code string) error { return &nativeaccess.Error{Status: status, Code: code} }

func (s *Service) principal(ctx context.Context, h http.Header) (weknora.Principal, error) {
	if !strings.HasPrefix(h.Get("Authorization"), "Bearer ") || h.Get("X-API-Key") != "" {
		return weknora.Principal{}, fail(401, "auth.human_required")
	}
	if s.Upstream == nil || s.Store == nil {
		return weknora.Principal{}, fail(503, "agent_icon.unavailable")
	}
	p, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return p, err
	}
	if p.User == nil || p.User.ID == "" || !p.User.IsActive || p.Tenant == nil || p.Tenant.ID == 0 {
		return p, fail(403, "identity.inactive")
	}
	if requested := h.Get("X-Tenant-ID"); requested != "" && requested != strconv.FormatUint(p.Tenant.ID, 10) {
		return p, fail(403, "workspace.denied")
	}
	role := roleFor(p)
	if role == "" {
		return p, fail(403, "workspace.denied")
	}
	if s.IdentityAllowed != nil {
		if err := s.IdentityAllowed(ctx, p.User.ID); err != nil {
			return p, err
		}
	}
	return p, nil
}
func roleFor(p weknora.Principal) string {
	for _, m := range p.Memberships {
		if m.TenantID == p.Tenant.ID {
			switch m.Role {
			case "owner", "admin", "contributor", "viewer":
				return m.Role
			}
		}
	}
	return ""
}

func (s *Service) Upload(ctx context.Context, h http.Header, raw []byte) (string, error) {
	p, err := s.principal(ctx, h)
	if err != nil {
		return "", err
	}
	if roleFor(p) == "viewer" {
		return "", fail(403, "agent_icon.edit_denied")
	}
	png, err := safeimage.Normalize(raw)
	if err != nil {
		return "", fail(400, "agent_icon.image_invalid")
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "", err
	}
	item := Image{ID: hex.EncodeToString(id[:]), TenantID: p.Tenant.ID, UserID: p.User.ID, PNG: png}
	if err = s.Store.Put(ctx, item); err != nil {
		if errors.Is(err, ErrQuota) {
			return "", fail(409, "agent_icon.quota_exceeded")
		}
		return "", err
	}
	return Prefix + item.ID, nil
}

func (s *Service) Read(ctx context.Context, h http.Header, id, agentID string) ([]byte, error) {
	p, err := s.principal(ctx, h)
	if err != nil {
		return nil, err
	}
	if !referenceID.MatchString(id) {
		return nil, fail(404, "agent_icon.not_found")
	}
	item, err := s.Store.Get(ctx, p.Tenant.ID, id)
	if errors.Is(err, ErrNotFound) {
		return nil, fail(404, "agent_icon.not_found")
	}
	if err != nil {
		return nil, err
	}
	// Before an Agent is saved, only the uploader can preview the draft image.
	// Attached-image reads always revalidate the Agent, including the uploader.
	if agentID == "" {
		if item.UserID != p.User.ID {
			return nil, fail(404, "agent_icon.not_found")
		}
	} else {
		if s.AgentAvatarForRead == nil {
			return nil, fail(503, "agent_icon.unavailable")
		}
		avatar, err := s.AgentAvatarForRead(ctx, agentID, h)
		if err != nil {
			return nil, err
		}
		if avatar != Prefix+id {
			return nil, fail(404, "agent_icon.not_found")
		}
	}
	return item.PNG, nil
}

// ValidateReference prevents users from attaching another user's private draft.
// A current visible Agent can keep or copy its own existing image; native Agent
// mutation authorization remains in force after this product validation.
func (s *Service) ValidateReference(ctx context.Context, h http.Header, agentID, avatar string) error {
	if !strings.HasPrefix(avatar, Prefix) {
		if strings.HasPrefix(avatar, "mc-avatar") {
			return fail(400, "agent_icon.reference_invalid")
		}
		return nil
	}
	id := strings.TrimPrefix(avatar, Prefix)
	p, err := s.principal(ctx, h)
	if err != nil {
		return err
	}
	if !referenceID.MatchString(id) {
		return fail(400, "agent_icon.reference_invalid")
	}
	item, err := s.Store.Get(ctx, p.Tenant.ID, id)
	if errors.Is(err, ErrNotFound) {
		return fail(404, "agent_icon.not_found")
	}
	if err != nil {
		return err
	}
	if item.UserID == p.User.ID {
		return nil
	}
	if agentID != "" && s.AgentAvatarForRead != nil {
		existing, err := s.AgentAvatarForRead(ctx, agentID, h)
		if err != nil {
			return err
		}
		if existing == avatar {
			return nil
		}
	}
	return fail(404, "agent_icon.not_found")
}
