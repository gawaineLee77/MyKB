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

	if strings.TrimSpace(cfg.ListenAddr) == "" || strings.TrimSpace(cfg.ProductVersion) == "" {
		return Config{}, fmt.Errorf("listen address and product version must not be empty")
	}
	return cfg, nil
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
