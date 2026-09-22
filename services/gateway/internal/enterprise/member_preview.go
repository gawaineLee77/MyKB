package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type MemberPreview struct {
	Email  string `json:"email"`
	State  string `json:"state"`
	UserID string `json:"user_id,omitempty"`
	Role   string `json:"role,omitempty"`
	Code   string `json:"code,omitempty"`
}

// PreviewMembers only resolves addresses supplied by a current workspace Owner.
// It performs no account/membership writes and forwards the human credential.
func (s *Service) PreviewMembers(ctx context.Context, p weknora.Principal, h http.Header, emails []string) ([]MemberPreview, error) {
	if h.Get("X-API-Key") != "" || h.Get("Authorization") == "" || p.User == nil || !p.User.IsActive || p.Tenant == nil {
		return nil, failure("workspace.owner_required", 403)
	}
	owner := false
	for _, m := range p.Memberships {
		if m.TenantID == p.Tenant.ID && m.Role == "owner" {
			owner = true
		}
	}
	if !owner {
		return nil, failure("workspace.owner_required", 403)
	}
	if len(emails) == 0 || len(emails) > 500 {
		return nil, failure("member.batch_invalid", 400)
	}
	if s.Identities == nil {
		return nil, failure("identity.unavailable", 503)
	}
	installation, err := s.Store.Installation(ctx)
	if err != nil {
		return nil, err
	}
	type member struct {
		Email string `json:"email"`
		ID    string `json:"user_id"`
		Role  string `json:"role"`
	}
	members := map[string]member{}
	complete := false
	for page := 1; page <= 500; page++ {
		raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%d/members?page=%d&page_size=100", p.Tenant.ID, page), h, nil)
		if err != nil {
			return nil, err
		}
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Members []member `json:"members"`
				Total   int      `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &result) != nil || !result.Success || result.Data.Total < 0 {
			return nil, failure("upstream.invalid_response", 502)
		}
		for _, m := range result.Data.Members {
			members[strings.ToLower(m.Email)] = m
		}
		if len(members) >= result.Data.Total {
			complete = true
			break
		}
		if len(result.Data.Members) == 0 {
			break
		}
	}
	if !complete {
		return nil, failure("member.preview_incomplete", 409)
	}
	out := make([]MemberPreview, 0, len(emails))
	for _, input := range emails {
		email := strings.ToLower(strings.TrimSpace(input))
		row := MemberPreview{Email: email, State: "ready"}
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || len(email) > 254 {
			row.State = "invalid"
			row.Code = "member.email_invalid"
			out = append(out, row)
			continue
		}
		alias, err := s.Identities.MemberAlias(ctx, email)
		if strings.EqualFold(email, installation.AdminEmail) {
			alias, err = installation.AdminEmail, nil
		}
		if err != nil {
			if !errors.Is(err, identity.ErrNotFound) {
				return nil, failure("identity.unavailable", 503)
			}
			row.State, row.Code = "unregistered", "member.account_not_registered"
		} else if m, ok := members[strings.ToLower(alias)]; ok {
			row.State, row.Role, row.UserID = "existing", m.Role, m.ID
		}
		out = append(out, row)
	}
	return out, nil
}
