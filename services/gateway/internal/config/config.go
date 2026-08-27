package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddr       = ":8080"
	defaultUpstreamURL      = "http://app:8080"
	defaultUpstreamVersion  = "v0.7.2"
	defaultUpstreamTimeout  = 5 * time.Second
	defaultRoutePolicyFile  = "config/phase1-route-policy.json"
	defaultCapabilitiesFile = "config/phase1-capabilities.json"
)

// Config contains only process and upstream-connection settings.
type Config struct {
	ListenAddr       string
	ProductVersion   string
	UpstreamURL      *url.URL
	UpstreamVersion  string
	UpstreamTimeout  time.Duration
	RoutePolicyFile  string
	CapabilitiesFile string
	DatabaseURL      string
}

// Load reads and validates gateway configuration from the environment.
func Load(buildVersion string) (Config, error) {
	cfg := Config{
		ListenAddr:       value("MINDCREEK_LISTEN_ADDR", defaultListenAddr),
		ProductVersion:   value("MINDCREEK_VERSION", buildVersion),
		UpstreamVersion:  value("MINDCREEK_UPSTREAM_VERSION", defaultUpstreamVersion),
		RoutePolicyFile:  value("MINDCREEK_ROUTE_POLICY_FILE", defaultRoutePolicyFile),
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

	if strings.TrimSpace(cfg.ListenAddr) == "" || strings.TrimSpace(cfg.ProductVersion) == "" {
		return Config{}, fmt.Errorf("listen address and product version must not be empty")
	}
	return cfg, nil
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
