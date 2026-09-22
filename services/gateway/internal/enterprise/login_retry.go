package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// v0.8.0 signs identical claims for two logins within one second. A just
// revoked token can therefore be reissued. Retry only a successful password
// login whose newly minted bearer fails native validation with 401. Every
// attempt still checks the password upstream; callers still verify identity.
// Refresh is intentionally excluded because it rotates a one-use credential.
func (s *Service) checkedPasswordLogin(ctx context.Context, input any) (json.RawMessage, weknora.Principal, error) {
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/login", nil, input)
		if err != nil {
			return nil, weknora.Principal{}, err
		}
		var result session
		if json.Unmarshal(raw, &result) != nil || !result.Success || result.Token == "" || result.RefreshToken == "" {
			return nil, weknora.Principal{}, failure("upstream.invalid_response", 502)
		}
		p, err := s.Upstream.CurrentPrincipal(ctx, Bearer(result.Token))
		var nativeErr *weknora.Error
		if attempt == 0 && errors.As(err, &nativeErr) && nativeErr.StatusCode == 401 {
			timer := time.NewTimer(1100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, weknora.Principal{}, ctx.Err()
			case <-timer.C:
				continue
			}
		}
		return raw, p, err
	}
	return nil, weknora.Principal{}, failure("admin.authentication_failed", 401)
}
