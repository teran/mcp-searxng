package config

import (
	"testing"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestLoad(t *testing.T) {
	t.Run("all env vars set correctly", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("LISTEN_ADDR", ":9090")
		t.Setenv("PROMETHEUS_METRICS_ADDR", ":9091")
		t.Setenv("RATE_LIMIT_GLOBAL", "200")
		t.Setenv("RATE_LIMIT_PER_CLIENT", "50")
		t.Setenv("WRITE_TIMEOUT", "600s")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.SearXNGURL != "http://searxng:8888" {
			t.Errorf("SearXNGURL = %q, want %q", cfg.SearXNGURL, "http://searxng:8888")
		}
		if cfg.ListenAddr != ":9090" {
			t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
		}
		if cfg.PrometheusMetricsAddr != ":9091" {
			t.Errorf("PrometheusMetricsAddr = %q, want %q", cfg.PrometheusMetricsAddr, ":9091")
		}
		if cfg.RateLimitGlobal != 200 {
			t.Errorf("RateLimitGlobal = %d, want 200", cfg.RateLimitGlobal)
		}
		if cfg.RateLimitPerClient != 50 {
			t.Errorf("RateLimitPerClient = %d, want 50", cfg.RateLimitPerClient)
		}
		if cfg.WriteTimeout != 600*time.Second {
			t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 600*time.Second)
		}
	})

	t.Run("defaults when env vars are not set", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.ListenAddr != ":8080" {
			t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
		}
		if cfg.PrometheusMetricsAddr != ":8081" {
			t.Errorf("PrometheusMetricsAddr = %q, want %q", cfg.PrometheusMetricsAddr, ":8081")
		}
		if cfg.RateLimitGlobal != 100 {
			t.Errorf("RateLimitGlobal = %d, want 100", cfg.RateLimitGlobal)
		}
		if cfg.RateLimitPerClient != 10 {
			t.Errorf("RateLimitPerClient = %d, want 10", cfg.RateLimitPerClient)
		}
		if cfg.WriteTimeout != 60*time.Second {
			t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 60*time.Second)
		}
	})
}

func TestLoad_Errors(t *testing.T) {
	t.Run("SEARXNG_URL is required", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for missing SEARXNG_URL")
		}
	})

	t.Run("SEARXNG_URL invalid URL", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "://invalid")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for invalid URL")
		}
	})

	t.Run("SEARXNG_URL must have http/https scheme", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "ftp://searxng:8888")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for non-http scheme")
		}
	})

	t.Run("SEARXNG_URL must have host", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for empty host")
		}
	})

	t.Run("RATE_LIMIT_GLOBAL must be >= 1", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("RATE_LIMIT_GLOBAL", "0")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for RATE_LIMIT_GLOBAL=0")
		}
	})

	t.Run("RATE_LIMIT_PER_CLIENT must be >= 1", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("RATE_LIMIT_PER_CLIENT", "-5")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for RATE_LIMIT_PER_CLIENT=-5")
		}
	})

	t.Run("envconfig error on invalid env var value", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("RATE_LIMIT_GLOBAL", "not-a-number")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for invalid RATE_LIMIT_GLOBAL value")
		}
	})

	t.Run("non-int value for validatePositiveInt", func(t *testing.T) {
		err := validation.Validate("not-an-int", validation.By(validatePositiveInt))
		if err == nil {
			t.Fatal("validatePositiveInt expected error for non-int value")
		}
	})

	t.Run("validateURLHost parse error", func(t *testing.T) {
		err := validateURLHost("://invalid")
		if err == nil {
			t.Fatal("validateURLHost expected error for unparseable URL")
		}
	})
}
