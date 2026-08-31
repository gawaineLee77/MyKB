package identity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

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
