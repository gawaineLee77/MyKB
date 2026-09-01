package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("MINDCREEK_LISTEN_ADDR", "")
	t.Setenv("MINDCREEK_UPSTREAM_URL", "")
	t.Setenv("MINDCREEK_UPSTREAM_TIMEOUT", "")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != ":8080" || cfg.ProductVersion != "test-version" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if got := cfg.UpstreamURL.String(); got != "http://app:8080" {
		t.Fatalf("UpstreamURL = %q", got)
	}
}

func TestLoadRejectsInvalidUpstream(t *testing.T) {
	t.Setenv("MINDCREEK_UPSTREAM_URL", "file:///etc/passwd")
	if _, err := Load("test-version"); err == nil {
		t.Fatal("Load() accepted a non-HTTP upstream URL")
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	t.Setenv("MINDCREEK_UPSTREAM_TIMEOUT", "0s")
	if _, err := Load("test-version"); err == nil {
		t.Fatal("Load() accepted a zero timeout")
	}
}

func TestLoadModelOverridePolicy(t *testing.T) {
	t.Setenv("MINDCREEK_MODEL_OVERRIDE_PROVIDERS", " OpenAI, generic,OPENAI ")
	t.Setenv("MINDCREEK_MODEL_OVERRIDE_HOSTS", " Models.Internal.Example, api.example.org ")
	t.Setenv("MINDCREEK_MODEL_OVERRIDE_ALLOW_HTTP", "true")
	t.Setenv("MINDCREEK_USER_MODEL_OVERRIDES", "true")
	cfg, err := Load("test-version")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModelOverrideProviders) != 2 || !cfg.ModelOverrideProviders["openai"] || !cfg.ModelOverrideHosts["models.internal.example"] || !cfg.ModelOverrideAllowHTTP || !cfg.ModelOverridesEnabled {
		t.Fatalf("model override policy = %+v", cfg)
	}
}

func TestLoadRejectsInvalidModelOverrideBooleans(t *testing.T) {
	t.Setenv("MINDCREEK_USER_MODEL_OVERRIDES", "sometimes")
	if _, err := Load("test-version"); err == nil {
		t.Fatal("Load() accepted an invalid override capability value")
	}
}

func TestLoadIdentityContract(t *testing.T) {
	t.Setenv("MINDCREEK_IDENTITY_ENABLED", "true")
	t.Setenv("MINDCREEK_IDENTITY_PROTOCOL", "oauth2")
	t.Setenv("MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP", "true")
	t.Setenv("MINDCREEK_EXTERNAL_ORIGIN", "http://mindcreek.internal:18080")
	t.Setenv("MINDCREEK_IDENTITY_ISSUER", "http://identity.internal")
	t.Setenv("MINDCREEK_IDENTITY_AUTHORIZATION_URL", "http://identity.internal/authorize")
	t.Setenv("MINDCREEK_IDENTITY_TOKEN_URL", "http://identity.internal/accesstoken")
	t.Setenv("MINDCREEK_IDENTITY_USERINFO_URL", "http://identity.internal/userinfo")
	t.Setenv("MINDCREEK_IDENTITY_REFRESH_URL", "http://identity.internal/refreshtoken")
	t.Setenv("MINDCREEK_IDENTITY_CLIENT_ID", "mindcreek")
	t.Setenv("MINDCREEK_IDENTITY_CLIENT_SECRET", "corporate-secret-value")
	t.Setenv("MINDCREEK_IDENTITY_AUTHORIZATION_METHOD", "POST")
	t.Setenv("MINDCREEK_IDENTITY_AUTHORIZATION_GRANT_TYPE", "authorization_code")
	t.Setenv("MINDCREEK_IDENTITY_PKCE_ENABLED", "false")
	t.Setenv("MINDCREEK_IDENTITY_STATE_REQUIRED", "false")
	t.Setenv("MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT", "query")
	t.Setenv("MINDCREEK_IDENTITY_SCOPES", "base.profile")
	t.Setenv("MINDCREEK_BROKER_CLIENT_SECRET", "broker-secret-value-with-32-characters")
	t.Setenv("MINDCREEK_IDENTITY_ALLOWED_EMPLOYEE_TYPES", " Employee, Contractor,employee ")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatal(err)
	}
	identity := cfg.Identity
	if !identity.Enabled || identity.Protocol != IdentityProtocolOAuth2 || identity.CallbackURL != "http://mindcreek.internal:18080/api/v1/mindcreek/oidc/callback" {
		t.Fatalf("identity contract = %+v", identity)
	}
	if identity.BrokerRedirectURI != "http://mindcreek.internal:18080/api/v1/auth/oidc/callback" ||
		!identity.AllowedEmployeeTypes["employee"] || !identity.AllowedEmployeeTypes["contractor"] ||
		identity.SubjectClaim != "globalUserID" || identity.TenantClaim != "tenantId" || !identity.SubjectTenantScoped ||
		identity.UsernameClaim != "uid" || identity.DisplayNameClaim != "uid" || identity.EmployeeTypeClaim != "employeeType" ||
		identity.AuthorizationMethod != "POST" || identity.AuthorizationGrant != "authorization_code" ||
		identity.PKCEEnabled || identity.StateRequired || identity.UserInfoTokenTransport != "query" ||
		identity.AuthorizationURL.String() != "http://identity.internal/authorize" ||
		identity.RefreshURL.String() != "http://identity.internal/refreshtoken" {
		t.Fatalf("identity mapping = %+v", identity)
	}
}

func TestLoadIdentityFailsClosed(t *testing.T) {
	base := map[string]string{
		"MINDCREEK_IDENTITY_ENABLED":           "true",
		"MINDCREEK_IDENTITY_PROTOCOL":          "oauth2",
		"MINDCREEK_EXTERNAL_ORIGIN":            "https://mindcreek.internal",
		"MINDCREEK_IDENTITY_ISSUER":            "https://identity.internal",
		"MINDCREEK_IDENTITY_AUTHORIZATION_URL": "https://identity.internal/authorize",
		"MINDCREEK_IDENTITY_TOKEN_URL":         "https://identity.internal/accesstoken",
		"MINDCREEK_IDENTITY_USERINFO_URL":      "https://identity.internal/userinfo",
		"MINDCREEK_IDENTITY_CLIENT_ID":         "mindcreek",
		"MINDCREEK_IDENTITY_CLIENT_SECRET":     "corporate-secret-value",
		"MINDCREEK_BROKER_CLIENT_SECRET":       "broker-secret-value-with-32-characters",
	}
	for name, value := range base {
		t.Setenv(name, value)
	}
	if _, err := Load("test-version"); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}

	t.Setenv("MINDCREEK_IDENTITY_TOKEN_URL", "")
	if _, err := Load("test-version"); err == nil {
		t.Fatal("OAuth2 identity contract accepted a missing token endpoint")
	}
}

func TestLoadDefaultsExistingIdentityToOIDCAndRequiresOpenIDScope(t *testing.T) {
	t.Setenv("MINDCREEK_IDENTITY_ENABLED", "true")
	t.Setenv("MINDCREEK_EXTERNAL_ORIGIN", "https://mindcreek.internal")
	t.Setenv("MINDCREEK_IDENTITY_ISSUER", "https://identity.internal")
	t.Setenv("MINDCREEK_IDENTITY_CLIENT_ID", "mindcreek")
	t.Setenv("MINDCREEK_IDENTITY_CLIENT_SECRET", "corporate-secret-value")
	t.Setenv("MINDCREEK_BROKER_CLIENT_SECRET", "broker-secret-value-with-32-characters")
	t.Setenv("MINDCREEK_IDENTITY_SCOPES", "profile,email")
	if _, err := Load("test-version"); err == nil {
		t.Fatal("OIDC identity contract accepted scopes without openid")
	}
	t.Setenv("MINDCREEK_IDENTITY_SCOPES", "openid,profile,email")
	cfg, err := Load("test-version")
	if err != nil || cfg.Identity.Protocol != IdentityProtocolOIDC {
		t.Fatalf("legacy identity default is not OIDC: protocol=%q error=%v", cfg.Identity.Protocol, err)
	}
}
