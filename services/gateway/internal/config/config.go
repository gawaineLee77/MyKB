package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr       = ":8080"
	defaultUpstreamURL      = "http://app:8080"
	defaultUpstreamVersion  = "v0.7.2"
	defaultUpstreamTimeout  = 5 * time.Second
	defaultRoutePolicyFile  = "config/phase1-route-policy.json"
	defaultRouteActionsFile = "config/phase2-route-actions.json"
	defaultCapabilitiesFile = "config/phase4-capabilities.json"
)

// Config contains only process and upstream-connection settings.
type Config struct {
	ListenAddr             string
	ProductVersion         string
	UpstreamURL            *url.URL
	UpstreamVersion        string
	UpstreamTimeout        time.Duration
	RoutePolicyFile        string
	RouteActionsFile       string
	CapabilitiesFile       string
	DatabaseURL            string
	ModelOverrideProviders map[string]bool
	ModelOverrideHosts     map[string]bool
	ModelOverrideAllowHTTP bool
	ModelOverridesEnabled  bool
	Identity               IdentityConfig
}

// IdentityConfig defines the closed-registration corporate OIDC contract.
// Corporate credentials are consumed only by the gateway identity broker;
// WeKnora is configured as a separate, private client of that broker.
type IdentityConfig struct {
	Enabled            bool
	AllowInsecureHTTP  bool
	ProviderName       string
	Issuer             string
	DiscoveryURL       *url.URL
	ClientID           string
	ClientSecret       string
	ExternalOrigin     *url.URL
	CallbackURL        string
	Scopes             []string
	UsernameClaim      string
	EmailClaim         string
	GroupClaim         string
	RequiredGroups     map[string]bool
	BrokerIssuer       string
	BrokerClientID     string
	BrokerClientSecret string
	BrokerRedirectURI  string
	BreakGlassUserIDs  map[string]bool
}

// Load reads and validates gateway configuration from the environment.
func Load(buildVersion string) (Config, error) {
	cfg := Config{
		ListenAddr:       value("MINDCREEK_LISTEN_ADDR", defaultListenAddr),
		ProductVersion:   value("MINDCREEK_VERSION", buildVersion),
		UpstreamVersion:  value("MINDCREEK_UPSTREAM_VERSION", defaultUpstreamVersion),
		RoutePolicyFile:  value("MINDCREEK_ROUTE_POLICY_FILE", defaultRoutePolicyFile),
		RouteActionsFile: value("MINDCREEK_ROUTE_ACTIONS_FILE", defaultRouteActionsFile),
		CapabilitiesFile: value("MINDCREEK_CAPABILITIES_FILE", defaultCapabilitiesFile),
		DatabaseURL:      optionalValue("MINDCREEK_DATABASE_URL"),
	}

	upstream, err := url.Parse(value("MINDCREEK_UPSTREAM_URL", defaultUpstreamURL))
	if err != nil || upstream.Scheme == "" || upstream.Host == "" || upstream.User != nil {
		return Config{}, fmt.Errorf("MINDCREEK_UPSTREAM_URL must be an absolute HTTP(S) URL without userinfo")
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return Config{}, fmt.Errorf("MINDCREEK_UPSTREAM_URL scheme must be http or https")
	}
	if upstream.RawQuery != "" || upstream.Fragment != "" {
		return Config{}, fmt.Errorf("MINDCREEK_UPSTREAM_URL must not include a query or fragment")
	}
	cfg.UpstreamURL = upstream

	timeout, err := time.ParseDuration(value("MINDCREEK_UPSTREAM_TIMEOUT", defaultUpstreamTimeout.String()))
	if err != nil || timeout <= 0 {
		return Config{}, fmt.Errorf("MINDCREEK_UPSTREAM_TIMEOUT must be a positive duration")
	}
	cfg.UpstreamTimeout = timeout

	cfg.ModelOverrideProviders = csvSet(value("MINDCREEK_MODEL_OVERRIDE_PROVIDERS", "generic,openai"))
	cfg.ModelOverrideHosts = csvSet(optionalValue("MINDCREEK_MODEL_OVERRIDE_HOSTS"))
	allowHTTP, err := strconv.ParseBool(value("MINDCREEK_MODEL_OVERRIDE_ALLOW_HTTP", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MINDCREEK_MODEL_OVERRIDE_ALLOW_HTTP must be true or false")
	}
	cfg.ModelOverrideAllowHTTP = allowHTTP
	overridesEnabled, err := strconv.ParseBool(value("MINDCREEK_USER_MODEL_OVERRIDES", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MINDCREEK_USER_MODEL_OVERRIDES must be true or false")
	}
	cfg.ModelOverridesEnabled = overridesEnabled

	identity, err := loadIdentityConfig()
	if err != nil {
		return Config{}, err
	}
	cfg.Identity = identity

	if strings.TrimSpace(cfg.ListenAddr) == "" || strings.TrimSpace(cfg.ProductVersion) == "" {
		return Config{}, fmt.Errorf("listen address and product version must not be empty")
	}
	return cfg, nil
}

func loadIdentityConfig() (IdentityConfig, error) {
	enabled, err := strconv.ParseBool(value("MINDCREEK_IDENTITY_ENABLED", "false"))
	if err != nil {
		return IdentityConfig{}, fmt.Errorf("MINDCREEK_IDENTITY_ENABLED must be true or false")
	}
	allowHTTP, err := strconv.ParseBool(value("MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP", "false"))
	if err != nil {
		return IdentityConfig{}, fmt.Errorf("MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP must be true or false")
	}
	result := IdentityConfig{
		Enabled:            enabled,
		AllowInsecureHTTP:  allowHTTP,
		ProviderName:       value("MINDCREEK_IDENTITY_PROVIDER_NAME", "Corporate account"),
		Issuer:             strings.TrimRight(optionalValue("MINDCREEK_IDENTITY_ISSUER"), "/"),
		ClientID:           optionalValue("MINDCREEK_IDENTITY_CLIENT_ID"),
		ClientSecret:       optionalValue("MINDCREEK_IDENTITY_CLIENT_SECRET"),
		Scopes:             csvList(value("MINDCREEK_IDENTITY_SCOPES", "openid,profile,email")),
		UsernameClaim:      value("MINDCREEK_IDENTITY_USERNAME_CLAIM", "preferred_username"),
		EmailClaim:         value("MINDCREEK_IDENTITY_EMAIL_CLAIM", "email"),
		GroupClaim:         value("MINDCREEK_IDENTITY_GROUP_CLAIM", "groups"),
		RequiredGroups:     csvSet(optionalValue("MINDCREEK_IDENTITY_REQUIRED_GROUPS")),
		BrokerClientID:     value("MINDCREEK_BROKER_CLIENT_ID", "mindcreek-weknora"),
		BrokerClientSecret: optionalValue("MINDCREEK_BROKER_CLIENT_SECRET"),
		BreakGlassUserIDs:  csvSet(optionalValue("MINDCREEK_BREAK_GLASS_USER_IDS")),
	}
	if !enabled {
		return result, nil
	}

	origin, err := parseIdentityURL("MINDCREEK_EXTERNAL_ORIGIN", optionalValue("MINDCREEK_EXTERNAL_ORIGIN"), allowHTTP)
	if err != nil {
		return IdentityConfig{}, err
	}
	if origin.Path != "" && origin.Path != "/" || origin.RawQuery != "" || origin.Fragment != "" || origin.User != nil {
		return IdentityConfig{}, fmt.Errorf("MINDCREEK_EXTERNAL_ORIGIN must contain only scheme and host")
	}
	origin.Path = ""
	result.ExternalOrigin = origin
	result.CallbackURL = origin.String() + "/api/v1/mindcreek/oidc/callback"
	result.BrokerIssuer = origin.String() + "/api/v1/mindcreek/oidc"
	result.BrokerRedirectURI = origin.String() + "/api/v1/auth/oidc/callback"

	issuer, err := parseIdentityURL("MINDCREEK_IDENTITY_ISSUER", result.Issuer, allowHTTP)
	if err != nil {
		return IdentityConfig{}, err
	}
	result.Issuer = strings.TrimRight(issuer.String(), "/")
	discoveryValue := optionalValue("MINDCREEK_IDENTITY_DISCOVERY_URL")
	if discoveryValue == "" {
		discoveryValue = result.Issuer + "/.well-known/openid-configuration"
	}
	result.DiscoveryURL, err = parseIdentityURL("MINDCREEK_IDENTITY_DISCOVERY_URL", discoveryValue, allowHTTP)
	if err != nil {
		return IdentityConfig{}, err
	}
	if result.ClientID == "" || len(result.ClientSecret) < 16 {
		return IdentityConfig{}, fmt.Errorf("corporate OIDC client ID and a client secret of at least 16 characters are required")
	}
	if len(result.BrokerClientSecret) < 32 {
		return IdentityConfig{}, fmt.Errorf("MINDCREEK_BROKER_CLIENT_SECRET must contain at least 32 characters")
	}
	if !contains(result.Scopes, "openid") {
		return IdentityConfig{}, fmt.Errorf("MINDCREEK_IDENTITY_SCOPES must include openid")
	}
	return result, nil
}

func parseIdentityURL(name, raw string, allowHTTP bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return nil, fmt.Errorf("%s must be an absolute HTTPS URL%s", name, map[bool]string{true: " (or HTTP when the development override is enabled)"}[allowHTTP])
	}
	return parsed, nil
}

func csvList(raw string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func csvSet(value string) map[string]bool {
	result := make(map[string]bool)
	for _, item := range strings.Split(value, ",") {
		if normalized := strings.ToLower(strings.TrimSpace(item)); normalized != "" {
			result[normalized] = true
		}
	}
	return result
}

func optionalValue(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func value(name, fallback string) string {
	if candidate, ok := os.LookupEnv(name); ok && strings.TrimSpace(candidate) != "" {
		return strings.TrimSpace(candidate)
	}
	return fallback
}
