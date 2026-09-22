package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// Characterizes the pre-R2 boundary. Onboarding must get its own narrow path;
// relaxing every business endpoint to accept tenantless users would be unsafe.
func TestR1BusinessPrincipalRejectsTenantlessUserIncludingSystemAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		resolver := principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return weknora.Principal{User: &weknora.User{ID: "r1-user", IsSystemAdmin: admin}}, nil
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases", nil)
		req.Header.Set("Authorization", "Bearer synthetic-r1")
		w := httptest.NewRecorder()
		if _, ok := resolvePrincipal(w, req, resolver); ok {
			t.Fatal("tenantless identity reached the business principal path")
		}
		assertErrorCode(t, w, http.StatusBadGateway, "auth.principal_invalid")
	}
}
