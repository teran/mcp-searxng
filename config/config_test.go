package config

import (
	"testing"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestLoad(t *testing.T) { //nolint:gocognit
	t.Run("all env vars set correctly", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("LISTEN_ADDR", ":9090")
		t.Setenv("INTERNAL_ADDR", ":9091")
		t.Setenv("MODE", "http")
		t.Setenv("RATE_LIMIT_GLOBAL", "200")
		t.Setenv("RATE_LIMIT_PER_CLIENT", "50")
		t.Setenv("WRITE_TIMEOUT", "600s")
		t.Setenv("LOG_LEVEL", "debug")
		t.Setenv("LOG_FORMAT", "json")
		t.Setenv("LOG_FILENAME", "/tmp/mcp-searxng.log")

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
		if cfg.InternalAddr != ":9091" {
			t.Errorf("InternalAddr = %q, want %q", cfg.InternalAddr, ":9091")
		}
		if cfg.Mode != "http" {
			t.Errorf("Mode = %q, want %q", cfg.Mode, "http")
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
		if cfg.LogLevel == nil || *cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %v, want %q", cfg.LogLevel, "debug")
		}
		if cfg.LogFormat != "json" {
			t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "json")
		}
		if cfg.LogFilename != "/tmp/mcp-searxng.log" {
			t.Errorf("LogFilename = %q, want %q", cfg.LogFilename, "/tmp/mcp-searxng.log")
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
		if cfg.InternalAddr != ":8081" {
			t.Errorf("InternalAddr = %q, want %q", cfg.InternalAddr, ":8081")
		}
		if cfg.Mode != "stdio" {
			t.Errorf("Mode = %q, want %q", cfg.Mode, "stdio")
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
		if cfg.LogLevel != nil {
			t.Errorf("LogLevel = %v, want nil (unset)", cfg.LogLevel)
		}
		if cfg.LogFormat != "text" {
			t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
		}
		if cfg.LogFilename != "" {
			t.Errorf("LogFilename = %q, want empty", cfg.LogFilename)
		}
	})
}

func TestLoad_Errors(t *testing.T) { //nolint:gocognit
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

	t.Run("LOG_LEVEL invalid", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("LOG_LEVEL", "verbose")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for invalid LOG_LEVEL")
		}
	})

	t.Run("LOG_LEVEL case-insensitive warning", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("LOG_LEVEL", "WARN")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		if cfg.LogLevel == nil || *cfg.LogLevel != "WARN" {
			t.Errorf("LogLevel = %v, want %q", cfg.LogLevel, "WARN")
		}
	})

	t.Run("LOG_FORMAT invalid", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("LOG_FORMAT", "xml")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for invalid LOG_FORMAT")
		}
	})

	t.Run("non-string value for validateLogLevel", func(t *testing.T) {
		err := validation.Validate(42, validation.By(validateLogLevel))
		if err == nil {
			t.Fatal("validateLogLevel expected error for non-string value")
		}
	})

	t.Run("non-string value for validateLogFormat", func(t *testing.T) {
		err := validation.Validate(42, validation.By(validateLogFormat))
		if err == nil {
			t.Fatal("validateLogFormat expected error for non-string value")
		}
	})

	t.Run("MODE invalid", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "http://searxng:8888")
		t.Setenv("MODE", "grpc")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() expected error for invalid MODE")
		}
	})

	t.Run("MODE non-string value", func(t *testing.T) {
		err := validation.Validate(42, validation.By(validateMode))
		if err == nil {
			t.Fatal("validateMode expected error for non-string value")
		}
	})
}

func TestLoad_InternalAddrLegacyShim(t *testing.T) {
	t.Setenv("SEARXNG_URL", "http://searxng:8888")

	t.Run("legacy variable used when INTERNAL_ADDR unset", func(t *testing.T) {
		t.Setenv("PROMETHEUS_METRICS_ADDR", ":9091")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		if cfg.InternalAddr != ":9091" {
			t.Errorf("InternalAddr = %q, want %q (legacy shim)", cfg.InternalAddr, ":9091")
		}
	})

	t.Run("INTERNAL_ADDR wins when both are set", func(t *testing.T) {
		t.Setenv("PROMETHEUS_METRICS_ADDR", ":9091")
		t.Setenv("INTERNAL_ADDR", ":9092")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		if cfg.InternalAddr != ":9092" {
			t.Errorf("InternalAddr = %q, want %q", cfg.InternalAddr, ":9092")
		}
	})

	t.Run("default used when neither set", func(t *testing.T) {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		if cfg.InternalAddr != ":8081" {
			t.Errorf("InternalAddr = %q, want %q", cfg.InternalAddr, ":8081")
		}
	})
}

func TestValidate(t *testing.T) {
	t.Run("valid config returns nil", func(t *testing.T) {
		cfg := Config{
			SearXNGURL:         "http://searxng:8888",
			ListenAddr:         ":8080",
			InternalAddr:       ":8081",
			RateLimitGlobal:    100,
			RateLimitPerClient: 10,
			WriteTimeout:       60,
			Mode:               "stdio",
			LogFormat:          "text",
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() returned error: %v", err)
		}
	})

	t.Run("invalid mode returns error", func(t *testing.T) {
		cfg := Config{
			SearXNGURL:         "http://searxng:8888",
			RateLimitGlobal:    100,
			RateLimitPerClient: 10,
			WriteTimeout:       60,
			Mode:               "grpc",
			LogFormat:          "text",
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() expected error for invalid mode, got nil")
		}
	})
}
