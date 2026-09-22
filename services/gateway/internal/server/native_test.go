package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

func TestNativeProductManagementRejectsMachineBeforeResolvingOwner(t *testing.T) {
	// No installation store or resolver: the credential-kind guard must reject
	// before any inherited facade can resolve an API key as a human Owner.
	h := newNativeHandler(config.Config{}, nil, Dependencies{}, http.NotFoundHandler())
	for _, path := range []string{"/api/v1/mindcreek/identities/employee/suspend", "/api/v1/mindcreek/models/builtin-mindcreek-chat/test", "/api/v1/mindcreek/models/overrides", "/api/v1/mindcreek/models"} {
		r := httptest.NewRequest("POST", path, strings.NewReader("{}"))
		r.Header.Set("X-API-Key", "synthetic-retrieve-key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "auth.human_required") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
