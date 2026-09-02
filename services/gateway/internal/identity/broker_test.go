package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type providerStub struct {
	state, nonce, challenge string
	method                  string
	claims                  Claims
	err                     error
}

func (p *providerStub) AuthorizationRequest(_ context.Context, state, nonce, challenge string) (AuthorizationRequest, error) {
	p.state, p.nonce, p.challenge = state, nonce, challenge
	if p.method == http.MethodPost {
		return AuthorizationRequest{Method: http.MethodPost, URL: "https://identity.example/authorize", Form: url.Values{"client_id": {"mindcreek"}}}, nil
	}
	return AuthorizationRequest{Method: http.MethodGet, URL: "https://identity.example/authorize?state=" + url.QueryEscape(state)}, nil
}

func (p *providerStub) Authenticate(_ context.Context, code, verifier, nonce string) (Claims, error) {
	if code != "corporate-code" || verifier == "" || nonce != p.nonce || p.challenge == "" {
		return Claims{}, ErrInvalid
	}
	return p.claims, p.err
}

func (*providerStub) EndSessionURL() string { return "https://identity.example/logout" }

type memoryStore struct {
	identity Identity
	audits   []AuditEvent
}

func (s *memoryStore) Upsert(_ context.Context, claims Claims, now time.Time) (Identity, error) {
	brokerSubject, email := stableAliases(claims.Issuer, claims.Subject)
	if s.identity.Issuer == "" {
		s.identity = Identity{
			Issuer: claims.Issuer, Subject: claims.Subject, BrokerSubject: brokerSubject,
			UpstreamEmail: email, CorporateEmail: claims.CorporateEmail, Username: claims.Username,
			DisplayName: claims.DisplayName, Groups: claims.Groups, Status: StatusActive,
			FirstSeenAt: now, LastSeenAt: now,
		}
	} else {
		s.identity.LastSeenAt = now
	}
	return s.identity, nil
}

func (s *memoryStore) GetByUpstreamEmail(_ context.Context, email string) (Identity, error) {
	if s.identity.UpstreamEmail != email {
		return Identity{}, ErrNotFound
	}
	return s.identity, nil
}

func (s *memoryStore) GetByBrokerSubject(_ context.Context, subject string) (Identity, error) {
	if s.identity.BrokerSubject != subject {
		return Identity{}, ErrNotFound
	}
	return s.identity, nil
}

func (s *memoryStore) BindLocalPrincipal(_ context.Context, email, userID string, tenantID uint64) error {
	if s.identity.UpstreamEmail != email {
		return ErrNotFound
	}
	s.identity.LocalUserID, s.identity.LocalTenantID = userID, tenantID
	return nil
}

func (s *memoryStore) SetStatus(_ context.Context, subject string, status Status, now time.Time) (Identity, error) {
	if s.identity.BrokerSubject != subject {
		return Identity{}, ErrNotFound
	}
	s.identity.Status = status
	if status == StatusSuspended {
		s.identity.SuspendedAt = &now
	} else {
		s.identity.SuspendedAt = nil
	}
	return s.identity, nil
}

func (s *memoryStore) RecordAudit(_ context.Context, event AuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}

func testIdentitySettings(t *testing.T) config.IdentityConfig {
	t.Helper()
	origin, err := url.Parse("https://mindcreek.example")
	if err != nil {
		t.Fatal(err)
	}
	return config.IdentityConfig{
		Enabled: true, Protocol: config.IdentityProtocolOIDC, ProviderName: "Example Identity", ExternalOrigin: origin,
		Issuer: "https://identity.example", ClientID: "mindcreek",
		AuthorizationMethod: http.MethodGet, AuthorizationGrant: "authorization_code",
		StateRequired: true, PKCEEnabled: true, UserInfoTokenTransport: "bearer",
		BrokerIssuer:   "https://mindcreek.example/api/v1/mindcreek/oidc",
		BrokerClientID: "mindcreek-weknora", BrokerClientSecret: "broker-secret-value-with-32-characters",
		BrokerRedirectURI: "https://mindcreek.example/api/v1/auth/oidc/callback",
	}
}

func TestBrokerPOSTAuthorizationAndCookieBoundCallback(t *testing.T) {
	settings := testIdentitySettings(t)
	settings.Protocol = config.IdentityProtocolOAuth2
	settings.AuthorizationMethod = http.MethodPost
	settings.StateRequired = false
	settings.PKCEEnabled = false
	settings.UserInfoTokenTransport = "query"
	provider := &providerStub{method: http.MethodPost, claims: Claims{
		Issuer: "https://identity.example", Subject: "tenant-user:employee-42", CorporateEmail: "mc-test@identity.invalid",
		Username: "alice", DisplayName: "Alice", Groups: []string{"employee-type:employee"},
	}}
	broker, err := NewBroker(settings, provider, &memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	authorize := httptest.NewRequest(http.MethodGet,
		"https://mindcreek.example/api/v1/mindcreek/oidc/authorize?response_type=code&client_id=mindcreek-weknora&redirect_uri="+
			url.QueryEscape("https://mindcreek.example/api/v1/auth/oidc/callback")+"&scope=openid+profile&state=upstream-state-value-1234", nil)
	recorder := httptest.NewRecorder()
	broker.ServeHTTP(recorder, authorize)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `method="post"`) ||
		!strings.Contains(recorder.Body.String(), `enctype="application/x-www-form-urlencoded"`) ||
		!strings.Contains(recorder.Header().Get("Content-Security-Policy"), "form-action https://identity.example") {
		t.Fatalf("POST authorization status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("authorization cookies=%v", cookies)
	}

	missingCookie := httptest.NewRecorder()
	broker.ServeHTTP(missingCookie, httptest.NewRequest(http.MethodGet,
		"https://mindcreek.example/api/v1/mindcreek/oidc/callback?code=corporate-code", nil))
	if missingCookie.Code != http.StatusBadRequest {
		t.Fatalf("cookie-less callback status=%d", missingCookie.Code)
	}
	callback := httptest.NewRequest(http.MethodGet,
		"https://mindcreek.example/api/v1/mindcreek/oidc/callback?code=corporate-code", nil)
	callback.AddCookie(cookies[0])
	callbackRecorder := httptest.NewRecorder()
	broker.ServeHTTP(callbackRecorder, callback)
	if callbackRecorder.Code != http.StatusFound {
		t.Fatalf("cookie-bound callback status=%d body=%s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
}

func TestBrokerAuthorizationCodeFlowAndReplayProtection(t *testing.T) {
	provider := &providerStub{claims: Claims{
		Issuer: "https://identity.example", Subject: "employee-42", CorporateEmail: "alice@example.com",
		Username: "alice", DisplayName: "Alice", Groups: []string{"knowledge"},
	}}
	store := &memoryStore{}
	broker, err := NewBroker(testIdentitySettings(t), provider, store)
	if err != nil {
		t.Fatal(err)
	}

	authorize := httptest.NewRequest(http.MethodGet,
		"https://mindcreek.example/api/v1/mindcreek/oidc/authorize?response_type=code&client_id=mindcreek-weknora&redirect_uri="+
			url.QueryEscape("https://mindcreek.example/api/v1/auth/oidc/callback")+"&scope=openid+profile&state=upstream-state-value-1234", nil)
	authorizeRecorder := httptest.NewRecorder()
	broker.ServeHTTP(authorizeRecorder, authorize)
	if authorizeRecorder.Code != http.StatusFound || provider.state == "" || provider.challenge == "" {
		t.Fatalf("authorize status=%d location=%q", authorizeRecorder.Code, authorizeRecorder.Header().Get("Location"))
	}
	cookies := authorizeRecorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("authorization cookie = %+v", cookies)
	}

	callback := httptest.NewRequest(http.MethodGet,
		"https://mindcreek.example/api/v1/mindcreek/oidc/callback?state="+url.QueryEscape(provider.state)+"&code=corporate-code", nil)
	callback.AddCookie(cookies[0])
	callbackRecorder := httptest.NewRecorder()
	broker.ServeHTTP(callbackRecorder, callback)
	if callbackRecorder.Code != http.StatusFound {
		t.Fatalf("callback status=%d body=%s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
	upstreamLocation, _ := url.Parse(callbackRecorder.Header().Get("Location"))
	brokerCode := upstreamLocation.Query().Get("code")
	if brokerCode == "" || upstreamLocation.Query().Get("state") != "upstream-state-value-1234" || len(store.audits) != 1 {
		t.Fatalf("callback location=%q audits=%d", upstreamLocation, len(store.audits))
	}

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {brokerCode},
		"redirect_uri": {"https://mindcreek.example/api/v1/auth/oidc/callback"},
	}
	tokenRequest := httptest.NewRequest(http.MethodPost, "/api/v1/mindcreek/oidc/token", strings.NewReader(form.Encode()))
	tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRequest.SetBasicAuth("mindcreek-weknora", "broker-secret-value-with-32-characters")
	tokenRecorder := httptest.NewRecorder()
	broker.ServeHTTP(tokenRecorder, tokenRequest)
	if tokenRecorder.Code != http.StatusOK {
		t.Fatalf("token status=%d body=%s", tokenRecorder.Code, tokenRecorder.Body.String())
	}
	var token map[string]any
	if err := json.Unmarshal(tokenRecorder.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	accessToken, _ := token["access_token"].(string)
	if accessToken == "" || token["id_token"] == "" {
		t.Fatalf("token response=%v", token)
	}

	userinfoRequest := httptest.NewRequest(http.MethodGet, "/api/v1/mindcreek/oidc/userinfo", nil)
	userinfoRequest.Header.Set("Authorization", "Bearer "+accessToken)
	userinfoRecorder := httptest.NewRecorder()
	broker.ServeHTTP(userinfoRecorder, userinfoRequest)
	if userinfoRecorder.Code != http.StatusOK || !strings.Contains(userinfoRecorder.Body.String(), "@identity.invalid") {
		t.Fatalf("userinfo status=%d body=%s", userinfoRecorder.Code, userinfoRecorder.Body.String())
	}

	replayRecorder := httptest.NewRecorder()
	broker.ServeHTTP(replayRecorder, tokenRequest.Clone(context.Background()))
	if replayRecorder.Code != http.StatusBadRequest {
		t.Fatalf("authorization code replay status=%d", replayRecorder.Code)
	}
}

func TestIdentityGateBindsAndRejectsSuspension(t *testing.T) {
	store := &memoryStore{}
	claims := Claims{Issuer: "https://identity.example", Subject: "employee-42", CorporateEmail: "alice@example.com", Username: "alice"}
	identity, err := store.Upsert(context.Background(), claims, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	gate, _ := NewGate(store, nil)
	principal := weknora.Principal{
		User:   &weknora.User{ID: "local-alice", Email: identity.UpstreamEmail, TenantID: 42},
		Tenant: &weknora.Tenant{ID: 42, Name: "Alice"},
	}
	if err := gate.Check(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if store.identity.LocalUserID != "local-alice" || store.identity.LocalTenantID != 42 {
		t.Fatalf("identity was not bound: %+v", store.identity)
	}
	admin, err := NewAdminService(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ChangeStatus(context.Background(), identity.BrokerSubject, StatusSuspended, "system-admin", "request-42", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if len(store.audits) != 1 || store.audits[0].Action != "identity.suspended" {
		t.Fatalf("suspension audit=%+v", store.audits)
	}
	if err := gate.Check(context.Background(), principal); err != ErrSuspended {
		t.Fatalf("suspended identity error=%v", err)
	}
}
