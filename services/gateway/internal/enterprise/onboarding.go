package enterprise

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type OnboardingStatus struct {
	State       string               `json:"state"`
	Memberships []weknora.Membership `json:"memberships"`
	ErrorCode   string               `json:"error_code,omitempty"`
	Retryable   bool                 `json:"retryable"`
}

func (s *Service) CheckEmployee(ctx context.Context, p weknora.Principal) (identity.Identity, error) {
	if s.Identities == nil || p.User == nil || p.User.ID == "" || !p.User.IsActive {
		return identity.Identity{}, failure("identity.unlinked", 403)
	}
	id, err := s.Identities.GetByUpstreamEmail(ctx, p.User.Email)
	if err != nil {
		return id, failure("identity.unlinked", 403)
	}
	if id.Status != identity.StatusActive {
		return id, failure("identity.suspended", 403)
	}
	if id.LocalUserID != "" && id.LocalUserID != p.User.ID {
		return id, failure("identity.unlinked", 403)
	}
	if id.LocalUserID == "" {
		if err := s.Identities.BindLocalAccount(ctx, id.UpstreamEmail, p.User.ID); err != nil {
			return id, failure("identity.unlinked", 403)
		}
		id.LocalUserID = p.User.ID
	}
	return id, nil
}

func (s *Service) Onboarding(ctx context.Context, h http.Header, join bool) (OnboardingStatus, error) {
	status := OnboardingStatus{State: "pending", Memberships: []weknora.Membership{}, Retryable: true}
	p, err := s.Upstream.CurrentPrincipal(ctx, identityHeaders(h))
	if err != nil {
		return status, err
	}
	id, err := s.CheckEmployee(ctx, p)
	if err != nil {
		return status, err
	}
	i, err := s.Store.Installation(ctx)
	if err != nil {
		return status, err
	}
	if i.Stage != "ready" {
		return status, failure("install.not_ready", 503)
	}
	err = s.Store.WithLock(ctx, "enterprise.employee."+id.BrokerSubject, func(store Store) error {
		// Refresh under the user lock: a concurrent request may have finished.
		p, err = s.Upstream.CurrentPrincipal(ctx, identityHeaders(h))
		if err != nil {
			return err
		}
		if _, err = s.CheckEmployee(ctx, p); err != nil {
			return err
		}
		status.Memberships = append([]weknora.Membership{}, p.Memberships...)
		o, err := store.Onboarding(ctx, id.BrokerSubject)
		if errors.Is(err, ErrNotFound) {
			o = OnboardingRecord{Subject: id.BrokerSubject, UserID: p.User.ID, TenantID: i.DefaultTenantID, State: "pending"}
		} else if err != nil {
			return err
		}
		if o.UserID != p.User.ID || o.TenantID != i.DefaultTenantID {
			return failure("onboarding.identity_conflict", 409)
		}
		if o.CompletedAt != nil {
			status.State = "ready"
			status.Retryable = false
			if memberRole(p, i.DefaultTenantID) == "" {
				status.State = "removed"
			}
			return nil
		}
		complete := func() error {
			now := time.Now().UTC()
			o.CompletedAt = &now
			o.State = "ready"
			o.OutcomeUnknown = false
			o.ErrorCode = ""
			if err := store.SaveOnboarding(ctx, o); err != nil {
				return err
			}
			status.State = "ready"
			status.Retryable = false
			status.Memberships = append([]weknora.Membership{}, p.Memberships...)
			return nil
		}
		// A role assigned by an Owner always wins over the default Viewer.
		if memberRole(p, i.DefaultTenantID) != "" {
			if join {
				return complete()
			}
			status.State = "pending"
			return nil
		}
		if o.OutcomeUnknown {
			status.State = "failed"
			status.ErrorCode = "onboarding.membership_result_unknown"
			status.Retryable = false
			return nil
		}
		if !join {
			status.State = o.State
			status.ErrorCode = o.ErrorCode
			return nil
		}
		secret, err := ReadSecret(i.MemberKeyRef)
		if err != nil {
			status.State = "failed"
			status.ErrorCode = "onboarding.member_key_unavailable"
			return nil
		}
		o.State = "joining"
		o.OutcomeUnknown = true
		o.ErrorCode = ""
		if err := store.SaveOnboarding(ctx, o); err != nil {
			return err
		}
		_, err = s.Upstream.EnterpriseRequest(ctx, "POST", tenantPath(i.DefaultTenantID)+"/members", keyHeaders(secret, i.DefaultTenantID), map[string]string{"email": id.UpstreamEmail, "role": "viewer"})
		if err != nil {
			var u *weknora.Error
			// Definitive rejections did not add a member. Network/5xx and 409
			// remain ambiguous until the actual membership is read below.
			if errors.As(err, &u) && (u.UpstreamStatus == 400 || u.UpstreamStatus == 401 || u.UpstreamStatus == 403 || u.UpstreamStatus == 404 || u.UpstreamStatus == 429) {
				o.OutcomeUnknown = false
			}
		}
		refreshed, readErr := s.Upstream.CurrentPrincipal(ctx, identityHeaders(h))
		if readErr == nil && refreshed.User != nil && refreshed.User.ID == o.UserID && memberRole(refreshed, i.DefaultTenantID) != "" {
			p = refreshed
			return complete()
		}
		o.State = "failed"
		o.ErrorCode = "onboarding.join_failed"
		if o.OutcomeUnknown {
			o.ErrorCode = "onboarding.membership_result_unknown"
		}
		if err := store.SaveOnboarding(ctx, o); err != nil {
			return err
		}
		status.State = o.State
		status.ErrorCode = o.ErrorCode
		status.Retryable = !o.OutcomeUnknown
		return nil
	})
	return status, err
}
