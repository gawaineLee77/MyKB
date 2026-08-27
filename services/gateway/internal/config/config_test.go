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
