package identity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestOAuth2ProviderMapsCorporateUserInfo(t *testing.T) {
	authorizationURL, _ := url.Parse("https://identity.example/authorize")
	tokenURL, _ := url.Parse("https://identity.example/accesstoken")
	userInfoURL, _ := url.Parse("https://identity.example/userinfo")
	settings := config.IdentityConfig{
		Enabled: true, Protocol: config.IdentityProtocolOAuth2, Issuer: "https://identity.example",
		AuthorizationURL: authorizationURL, TokenURL: tokenURL, UserInfoURL: userInfoURL,
		ClientID: "mindcreek", ClientSecret: "corporate-secret-value", ClientAuthMethod: "client_secret_post",
		AuthorizationMethod: http.MethodPost, AuthorizationGrant: "authorization_code", AuthorizationDisplay: "page",
		TokenRequestFormat: "json", StateRequired: false, UserInfoTokenTransport: "query",
		CallbackURL: "https://mindcreek.example/api/v1/mindcreek/oidc/callback", CorporateRedirectURI: "https://mindcreek.example",
		Scopes: []string{"base.profile"}, SubjectClaim: "globalUserID", TenantClaim: "tenantId", SubjectTenantScoped: true,
		UsernameClaim: "uid", DisplayNameClaim: "uid", UUIDClaim: "uuid", EmployeeTypeClaim: "employeeType",
		AllowedEmployeeTypes: map[string]bool{"employee": true},
	}
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		switch request.URL.Path {
		case "/accesstoken":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("token request method=%s headers=%v", request.Method, request.Header)
			}
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["client_id"] != "mindcreek" || payload["client_secret"] != "corporate-secret-value" ||
				payload["grant_type"] != "authorization_code" || payload["code"] != "corporate-code" ||
				payload["redirect_uri"] != "https://mindcreek.example" || payload["code_verifier"] != "" {
				t.Fatalf("token payload=%v", payload)
			}
			return jsonResponse(http.StatusOK, `{"access_token":"corporate-access","token_type":"Bearer","refresh_token":"unused","scope":"base.profile"}`), nil
		case "/userinfo":
			if request.Header.Get("Authorization") != "" || request.URL.Query().Get("access_token") != "corporate-access" ||
				request.URL.Query().Get("client_id") != "mindcreek" || request.URL.Query().Get("scope") != "base.profile" {
				t.Fatalf("userinfo request URL=%s authorization=%q", request.URL, request.Header.Get("Authorization"))
			}
			return jsonResponse(http.StatusOK, `{"employeeType":"Employee","globalUserID":"global-42","tenantId":"tenant-a","uid":"alice","uuid":"uuid-42"}`), nil
		default:
			t.Fatalf("unexpected request URL %s", request.URL)
			return nil, nil
		}
	})}
	provider, err := NewOAuth2Provider(settings, client)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := provider.AuthorizationRequest(context.Background(), "state-value", "unused-nonce", "challenge-value")
	if err != nil {
		t.Fatal(err)
	}
	if authorization.Method != http.MethodPost || authorization.URL != authorizationURL.String() ||
		authorization.Form.Get("response_type") != "code" || authorization.Form.Get("display") != "page" ||
		authorization.Form.Get("state") != "" || authorization.Form.Get("redirect_uri") != settings.CorporateRedirectURI ||
		authorization.Form.Get("code_challenge") != "" || authorization.Form.Get("scope") != "base.profile" {
		t.Fatalf("authorization request=%+v", authorization)
	}
	claims, err := provider.Authenticate(context.Background(), "corporate-code", "verifier-value", "unused-nonce")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || claims.Subject == "global-42" || !strings.HasPrefix(claims.Subject, "tenant-user:") ||
		claims.Username != "alice" || claims.DisplayName != "alice" ||
		!strings.HasSuffix(claims.CorporateEmail, "@identity.invalid") ||
		len(claims.Groups) != 1 || claims.Groups[0] != "employee-type:employee" {
		t.Fatalf("mapped claims=%+v requests=%d", claims, requests)
	}
}

func TestOAuth2ProviderRejectsUnapprovedEmployeeType(t *testing.T) {
	endpoint, _ := url.Parse("https://identity.example/endpoint")
	settings := config.IdentityConfig{
		Enabled: true, Protocol: config.IdentityProtocolOAuth2, Issuer: "https://identity.example",
		AuthorizationURL: endpoint, TokenURL: endpoint, UserInfoURL: endpoint,
		ClientID: "mindcreek", ClientSecret: "corporate-secret-value", ClientAuthMethod: "client_secret_basic",
		AuthorizationMethod: http.MethodGet, AuthorizationGrant: "authorization_code", TokenRequestFormat: "form",
		UserInfoTokenTransport: "bearer", CallbackURL: "https://mindcreek.example/api/v1/mindcreek/oidc/callback",
		CorporateRedirectURI: "https://mindcreek.example/api/v1/mindcreek/oidc/callback",
		SubjectClaim:         "globalUserID", TenantClaim: "tenantId", SubjectTenantScoped: true,
		UsernameClaim: "uid", DisplayNameClaim: "uid", EmployeeTypeClaim: "employeeType",
		AllowedEmployeeTypes: map[string]bool{"employee": true},
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			username, password, ok := request.BasicAuth()
			if !ok || username != settings.ClientID || password != settings.ClientSecret {
				t.Fatal("client_secret_basic was not used")
			}
			return jsonResponse(http.StatusOK, `{"access_token":"corporate-access","token_type":"bearer"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"employeeType":"external","globalUserID":"global-42","tenantId":"tenant-a","uid":"alice","uuid":"uuid-42"}`), nil
	})}
	provider, err := NewOAuth2Provider(settings, client)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := provider.Authenticate(context.Background(), "corporate-code", "", "")
	if err != ErrEmployeeTypeDenied || claims.Subject == "" {
		t.Fatalf("claims=%+v error=%v", claims, err)
	}
}

func TestOAuth2ProviderRejectsMissingStableUserInfo(t *testing.T) {
	endpoint, _ := url.Parse("https://identity.example/endpoint")
	settings := config.IdentityConfig{
		Enabled: true, Protocol: config.IdentityProtocolOAuth2, Issuer: "https://identity.example",
		AuthorizationURL: endpoint, TokenURL: endpoint, UserInfoURL: endpoint,
		ClientID: "mindcreek", ClientSecret: "corporate-secret-value", ClientAuthMethod: "client_secret_post",
		AuthorizationMethod: http.MethodGet, AuthorizationGrant: "authorization_code", TokenRequestFormat: "form",
		UserInfoTokenTransport: "bearer", CallbackURL: "https://mindcreek.example/api/v1/mindcreek/oidc/callback",
		CorporateRedirectURI: "https://mindcreek.example/api/v1/mindcreek/oidc/callback",
		SubjectClaim:         "globalUserID", TenantClaim: "tenantId", SubjectTenantScoped: true,
		UsernameClaim: "uid", DisplayNameClaim: "uid", UUIDClaim: "uuid", EmployeeTypeClaim: "employeeType",
	}
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"employeeType":"employee","tenantId":"tenant-a","uid":"alice","uuid":"uuid-42"}`), nil
	})}
	provider, err := NewOAuth2Provider(settings, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.loadClaims(context.Background(), "corporate-access", "base.profile"); err == nil ||
		!strings.Contains(err.Error(), "stable identity") {
		t.Fatalf("missing globalUserID error=%v", err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCorporateIDTokenSecurityMatrix(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwks" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "corporate-key", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()

	discoveryURL, _ := url.Parse(server.URL + "/discovery")
	settings := config.IdentityConfig{
		Enabled: true, AllowInsecureHTTP: true, Issuer: "https://identity.example",
		DiscoveryURL: discoveryURL, ClientID: "mindcreek-client", ClientSecret: "corporate-secret-value",
	}
	provider, err := NewCorporateProvider(settings, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	metadata := &providerMetadata{JWKSURI: server.URL + "/jwks"}
	now := time.Now().UTC().Truncate(time.Second)
	valid := map[string]any{
		"iss": settings.Issuer, "sub": "employee-42", "aud": settings.ClientID, "nonce": "nonce-42",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "valid"},
		{name: "wrong issuer", mutate: func(c map[string]any) { c["iss"] = "https://attacker.example" }},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = "another-client" }},
		{name: "wrong authorized party", mutate: func(c map[string]any) {
			c["aud"] = []string{settings.ClientID, "another-client"}
			c["azp"] = "another-client"
		}},
		{name: "wrong nonce", mutate: func(c map[string]any) { c["nonce"] = "replayed-nonce" }},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = now.Add(-time.Minute).Unix() }},
		{name: "missing subject", mutate: func(c map[string]any) { delete(c, "sub") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := make(map[string]any, len(valid))
			for name, value := range valid {
				claims[name] = value
			}
			if tc.mutate != nil {
				tc.mutate(claims)
			}
			raw := signProviderJWT(t, key, claims)
			_, err := provider.verifyIDToken(context.Background(), metadata, raw, "nonce-42", now)
			if tc.name == "valid" && err != nil {
				t.Fatalf("valid token rejected: %v", err)
			}
			if tc.name != "valid" && err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}

func signProviderJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "corporate-key", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
