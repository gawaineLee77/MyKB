// Package ownership resolves the canonical owner of an upstream knowledge base.
package ownership

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var (
	ErrInvalid          = errors.New("invalid ownership request")
	ErrNotFound         = errors.New("knowledge base not found")
	ErrUnavailable      = errors.New("knowledge-base ownership is unavailable")
	ErrAdoptionRequired = errors.New("knowledge base requires explicit owner adoption")
	ErrConflict         = errors.New("product and upstream ownership conflict")
)

type Source string

const (
	SourceProductProfile Source = "product_profile"
	SourceUpstream       Source = "upstream_creator"
)

type Ownership struct {
	KnowledgeBaseID string
	OwnerUserID     string
	TenantID        uint64
	ProductMode     profile.ProductMode
	Source          Source
}

func (o Ownership) IsPersonalNotes() bool {
	return o.ProductMode == profile.ModePersonalNotes
}

type ProfileStore interface {
	Get(context.Context, string) (profile.Profile, error)
}

type Upstream interface {
	GetKnowledgeBase(context.Context, string, http.Header) (weknora.KnowledgeBase, error)
}

type Resolver struct {
	profiles ProfileStore
	upstream Upstream
}

func NewResolver(profiles ProfileStore, upstream Upstream) (*Resolver, error) {
	if profiles == nil || upstream == nil {
		return nil, fmt.Errorf("profile store and upstream ownership source are required")
	}
	return &Resolver{profiles: profiles, upstream: upstream}, nil
}

func (r *Resolver) Resolve(ctx context.Context, kbID string, inbound http.Header) (Ownership, error) {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return Ownership{}, ErrInvalid
	}
	kb, err := r.upstream.GetKnowledgeBase(ctx, kbID, inbound)
	if err != nil {
		var upstreamError *weknora.Error
		if errors.As(err, &upstreamError) &&
			(upstreamError.Code == "upstream.not_found" || upstreamError.StatusCode == http.StatusForbidden) {
			// An upstream 403 commonly means that the caller is in another
			// workspace. Treat it exactly like a missing KB so neither the
			// ownership resolver nor the public gateway discloses existence.
			return Ownership{}, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return Ownership{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if kb.ID != kbID || kb.TenantID == 0 {
		return Ownership{}, fmt.Errorf("%w: incomplete upstream knowledge base", ErrUnavailable)
	}
	if strings.TrimSpace(kb.CreatorID) == "" {
		return Ownership{}, ErrAdoptionRequired
	}

	kbProfile, err := r.profiles.Get(ctx, kbID)
	if errors.Is(err, profile.ErrNotFound) {
		return Ownership{
			KnowledgeBaseID: kb.ID,
			OwnerUserID:     kb.CreatorID,
			TenantID:        kb.TenantID,
			Source:          SourceUpstream,
		}, nil
	}
	if err != nil {
		return Ownership{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if kbProfile.UpstreamKBID != kb.ID || kbProfile.OwnerUserID != kb.CreatorID || kbProfile.TenantID != kb.TenantID {
		return Ownership{}, ErrConflict
	}
	switch kbProfile.ProductMode {
	case profile.ModePersonalNotes, profile.ModeRAG, profile.ModeOntology:
	default:
		return Ownership{}, fmt.Errorf("%w: unsupported product mode", ErrConflict)
	}
	return Ownership{
		KnowledgeBaseID: kb.ID,
		OwnerUserID:     kbProfile.OwnerUserID,
		TenantID:        kbProfile.TenantID,
		ProductMode:     kbProfile.ProductMode,
		Source:          SourceProductProfile,
	}, nil
}
