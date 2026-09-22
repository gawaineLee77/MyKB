package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
)

func TestCallbackDiagnosticsPreserveRejectionsAndRedactCredentials(t *testing.T) {
	for _, scenario := range []string{"state_missing", "cookie_missing", "cookie_mismatch", "state_mismatch", "transaction_not_found", "transaction_expired", "accepted_without_state"} {
		t.Run(scenario, func(t *testing.T) {
			settings := testIdentitySettings(t)
			settings.ClientSecret = "CLIENT_SECRET_CANARY"
			if scenario == "accepted_without_state" {
				settings.Protocol = config.IdentityProtocolOAuth2
				settings.StateRequired = false
			}
			provider := &providerStub{claims: Claims{Issuer: "https://identity.example", Subject: "subject-canary", Username: "private-name", CorporateEmail: "private-email@example.test"}}
			broker, err := NewBroker(settings, provider, &memoryStore{})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			ctx := diagnostics.WithLogger(context.Background(), log.New(&output, "", 0))
			broker.logConfiguration(ctx)
			authorize := httptest.NewRequest("GET", "https://mindcreek.example/api/v1/mindcreek/oidc/authorize?response_type=code&client_id=mindcreek-weknora&scope=openid&state=UPSTREAM_STATE_CANARY&redirect_uri="+url.QueryEscape(settings.BrokerRedirectURI), nil).WithContext(ctx)
			authorize.Header.Set("X-Request-ID", "authorize-request")
			started := httptest.NewRecorder()
			broker.ServeHTTP(started, authorize)
			cookie := started.Result().Cookies()[0]
			originalCookie := cookie.Value
			state := provider.state
			switch scenario {
			case "state_missing", "accepted_without_state":
				state = ""
			case "cookie_mismatch":
				cookie.Value = "OTHER_COOKIE_CANARY"
			case "state_mismatch":
				state = "WRONG_STATE_CANARY"
			case "transaction_not_found":
				broker, err = NewBroker(settings, provider, &memoryStore{})
				if err != nil {
					t.Fatal(err)
				}
			case "transaction_expired":
				broker.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
			}
			callback := httptest.NewRequest("GET", "https://mindcreek.example/api/v1/mindcreek/oidc/callback?code=corporate-code&state="+url.QueryEscape(state), nil).WithContext(ctx)
			callback.Header.Set("X-Request-ID", "callback-request")
			if scenario != "cookie_missing" {
				callback.AddCookie(cookie)
			}
			finished := httptest.NewRecorder()
			broker.ServeHTTP(finished, callback)
			reason := scenario
			if scenario == "accepted_without_state" {
				reason = "accepted"
				if finished.Code != 302 || provider.authenticateCalls != 1 {
					t.Fatal("documented cookie-bound compatibility flow failed")
				}
			} else if finished.Code != 400 || provider.authenticateCalls != 0 || !strings.Contains(finished.Body.String(), "invalid_state") {
				t.Fatal("diagnostics changed the public rejection or called provider before authorization")
			}
			var checked map[string]any
			var flow string
			for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
				var event map[string]any
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatal(err)
				}
				if event["event"] == "identity_authorize_started" {
					flow = event["flow_id"].(string)
				}
				if event["event"] == "identity_callback_checked" {
					checked = event
				}
			}
			if checked["reason"] != reason || checked["request_id"] != "callback-request" {
				t.Fatalf("unexpected callback diagnostic: %+v", checked)
			}
			if scenario != "transaction_not_found" && checked["flow_id"] != flow {
				t.Fatal("flow correlation was lost")
			}
			for _, secret := range []string{settings.ClientSecret, settings.BrokerClientSecret, originalCookie, provider.state, provider.nonce, provider.challenge, "UPSTREAM_STATE_CANARY", "corporate-code", "private-name", "private-email@example.test", "WRONG_STATE_CANARY", "OTHER_COOKIE_CANARY"} {
				if secret != "" && strings.Contains(output.String(), secret) {
					t.Fatal("diagnostics exposed a credential or employee value")
				}
			}
		})
	}
}

func TestPhase5OAuthContractWithTLSAndDiagnosticCorrelation(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "untrusted_ca", true: "trusted_ca"}[trusted], func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/token":
					var body map[string]string
					if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || body["client_secret"] != "short-secret" || body["code"] != "PRIVATE_CODE" || body["code_verifier"] != "" || body["redirect_uri"] != "https://mindcreek.example" {
						t.Error("Phase 5 token request contract changed")
					}
					_, _ = io.WriteString(w, `{"access_token":"PRIVATE_ACCESS","scope":"base.profile"}`)
				case "/userinfo":
					if r.Method != "GET" || r.URL.Query().Get("access_token") != "PRIVATE_ACCESS" || r.URL.Query().Get("scope") != "base.profile" || r.Header.Get("Authorization") != "" {
						t.Error("Phase 5 UserInfo request contract changed")
					}
					_, _ = io.WriteString(w, `{"globalUserID":"PRIVATE_SUBJECT","tenantId":"PRIVATE_TENANT","uid":"PRIVATE_NAME","uuid":"PRIVATE_UUID","employeeType":"Employee"}`)
				default:
					t.Error("unexpected provider endpoint")
				}
			}))
			defer server.Close()
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			settings := testIdentitySettings(t)
			settings.Protocol = config.IdentityProtocolOAuth2
			settings.Issuer = server.URL
			settings.AuthorizationURL, _ = url.Parse(server.URL + "/authorize")
			settings.TokenURL, _ = url.Parse(server.URL + "/token")
			settings.UserInfoURL, _ = url.Parse(server.URL + "/userinfo")
			settings.CorporateRedirectURI = "https://mindcreek.example"
			settings.ClientSecret = "short-secret"
			settings.ClientAuthMethod = "client_secret_post"
			settings.AuthorizationMethod = "POST"
			settings.TokenRequestFormat = "json"
			settings.UserInfoTokenTransport = "query"
			settings.StateRequired, settings.PKCEEnabled = false, false
			settings.SubjectClaim, settings.TenantClaim, settings.UsernameClaim = "globalUserID", "tenantId", "uid"
			settings.DisplayNameClaim, settings.UUIDClaim, settings.EmployeeTypeClaim = "uid", "uuid", "employeeType"
			settings.SubjectTenantScoped = true
			client := &http.Client{}
			if trusted {
				client = server.Client()
			}
			provider, err := NewOAuth2Provider(settings, client)
			if err != nil {
				t.Fatal(err)
			}
			store := &memoryStore{}
			broker, err := NewBroker(settings, provider, store)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			ctx := diagnostics.WithLogger(context.Background(), log.New(&output, "", 0))
			authorize := httptest.NewRequest("GET", "https://mindcreek.example/api/v1/mindcreek/oidc/authorize?response_type=code&client_id=mindcreek-weknora&scope=openid&state=private-upstream-state&redirect_uri="+url.QueryEscape(settings.BrokerRedirectURI), nil).WithContext(ctx)
			started := httptest.NewRecorder()
			broker.ServeHTTP(started, authorize)
			if started.Code != 200 || !strings.Contains(started.Body.String(), `method="post"`) || strings.Contains(started.Body.String(), `name="state"`) {
				t.Fatal("Phase 5 authorization form changed")
			}
			callback := httptest.NewRequest("GET", "https://mindcreek.example/api/v1/mindcreek/oidc/callback?code=PRIVATE_CODE", nil).WithContext(ctx)
			callback.Header.Set("X-Request-ID", "callback-flow-test")
			callback.AddCookie(started.Result().Cookies()[0])
			finished := httptest.NewRecorder()
			broker.ServeHTTP(finished, callback)
			if finished.Code != 302 {
				t.Fatal("cookie-bound callback was rejected")
			}
			if trusted {
				if calls != 2 || store.identity.LocalUserID != "" || !strings.Contains(output.String(), `"event":"identity_callback_completed"`) {
					t.Fatal("trusted provider flow did not complete the broker handoff")
				}
			} else if calls != 0 || !strings.Contains(output.String(), `"error_kind":"tls_unknown_authority"`) || !strings.Contains(finished.Header().Get("Location"), "identity.validation_failed") {
				t.Fatal("untrusted CA failure was not classified without changing the public contract")
			}
			for _, forbidden := range []string{"PRIVATE_CODE", "PRIVATE_ACCESS", "PRIVATE_SUBJECT", "PRIVATE_TENANT", "PRIVATE_NAME", "PRIVATE_UUID", "short-secret", "private-upstream-state", "access_token="} {
				if strings.Contains(output.String(), forbidden) {
					t.Fatal("provider diagnostic exposed confidential values")
				}
			}
		})
	}
}
