package ownership

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type profileStub struct {
	result profile.Profile
	err    error
}

func (s profileStub) Get(context.Context, string) (profile.Profile, error) {
	return s.result, s.err
}

type upstreamStub struct {
	result       weknora.KnowledgeBase
	err          error
	seenHeaderID string
}

func (s *upstreamStub) GetKnowledgeBase(_ context.Context, _ string, header http.Header) (weknora.KnowledgeBase, error) {
	s.seenHeaderID = header.Get("X-Request-ID")
	return s.result, s.err
}

func TestResolverUsesMatchingProductProfile(t *testing.T) {
	upstream := &upstreamStub{result: weknora.KnowledgeBase{ID: "kb-1", TenantID: 42, CreatorID: "alice"}}
	resolver := mustResolver(t, profileStub{result: profile.Profile{
		UpstreamKBID: "kb-1", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModeRAG,
	}}, upstream)
	header := make(http.Header)
	header.Set("X-Request-ID", "request-1")
	result, err := resolver.Resolve(context.Background(), "kb-1", header)
	if err != nil {
		t.Fatal(err)
	}
	if result.OwnerUserID != "alice" || result.TenantID != 42 || result.ProductMode != profile.ModeRAG || result.Source != SourceProductProfile {
		t.Fatalf("ownership = %+v", result)
	}
	if upstream.seenHeaderID != "request-1" {
		t.Fatalf("request header was not forwarded: %q", upstream.seenHeaderID)
	}
}

func TestResolverUsesUpstreamCreatorForUnprofiledKB(t *testing.T) {
	resolver := mustResolver(t, profileStub{err: profile.ErrNotFound}, &upstreamStub{result: weknora.KnowledgeBase{
		ID: "kb-legacy", TenantID: 7, CreatorID: "legacy-owner",
	}})
	result, err := resolver.Resolve(context.Background(), "kb-legacy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != SourceUpstream || result.OwnerUserID != "legacy-owner" || result.ProductMode != "" {
		t.Fatalf("ownership = %+v", result)
	}
}

func TestResolverFailsClosedForOwnerlessOrConflictingKB(t *testing.T) {
	tests := []struct {
		name     string
		profiles profileStub
		upstream *upstreamStub
		want     error
	}{
		{
			name:     "ownerless legacy",
			profiles: profileStub{err: profile.ErrNotFound},
			upstream: &upstreamStub{result: weknora.KnowledgeBase{ID: "kb-1", TenantID: 42}},
			want:     ErrAdoptionRequired,
		},
		{
			name: "owner mismatch",
			profiles: profileStub{result: profile.Profile{
				UpstreamKBID: "kb-1", TenantID: 42, OwnerUserID: "bob", ProductMode: profile.ModeRAG,
			}},
			upstream: &upstreamStub{result: weknora.KnowledgeBase{ID: "kb-1", TenantID: 42, CreatorID: "alice"}},
			want:     ErrConflict,
		},
		{
			name:     "profile unavailable",
			profiles: profileStub{err: errors.New("database offline")},
			upstream: &upstreamStub{result: weknora.KnowledgeBase{ID: "kb-1", TenantID: 42, CreatorID: "alice"}},
			want:     ErrUnavailable,
		},
		{
			name:     "upstream missing",
			profiles: profileStub{err: profile.ErrNotFound},
			upstream: &upstreamStub{err: &weknora.Error{Code: "upstream.not_found", StatusCode: http.StatusNotFound}},
			want:     ErrNotFound,
		},
		{
			name:     "upstream hides cross workspace resource",
			profiles: profileStub{err: profile.ErrNotFound},
			upstream: &upstreamStub{err: &weknora.Error{Code: "upstream.forbidden", StatusCode: http.StatusForbidden}},
			want:     ErrNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := mustResolver(t, test.profiles, test.upstream)
			_, err := resolver.Resolve(context.Background(), "kb-1", nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("Resolve() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestResolverRejectsEmptyID(t *testing.T) {
	resolver := mustResolver(t, profileStub{}, &upstreamStub{})
	if _, err := resolver.Resolve(context.Background(), " ", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func mustResolver(t *testing.T, profiles ProfileStore, upstream Upstream) *Resolver {
	t.Helper()
	resolver, err := NewResolver(profiles, upstream)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}
