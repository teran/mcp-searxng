// Package config provides configuration loading from environment variables
// using kelseyhightower/envconfig and validation via ozzo-validation.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/kelseyhightower/envconfig"

	"github.com/teran/mcp-searxng/logging"
)

// Config represents the application configuration loaded from environment variables.
type Config struct {
	SearXNGURL         string        `envconfig:"SEARXNG_URL" required:"true"`
	ListenAddr         string        `envconfig:"LISTEN_ADDR" default:":8080"`
	InternalAddr       string        `envconfig:"INTERNAL_ADDR" default:":8081"`
	RateLimitGlobal    int           `envconfig:"RATE_LIMIT_GLOBAL" default:"100"`
	RateLimitPerClient int           `envconfig:"RATE_LIMIT_PER_CLIENT" default:"10"`
	WriteTimeout       time.Duration `envconfig:"WRITE_TIMEOUT" default:"60s"`
	Mode               string        `envconfig:"MODE" default:"stdio"`
	LogLevel           *string       `envconfig:"LOG_LEVEL"`
	LogFormat          string        `envconfig:"LOG_FORMAT" default:"text"`
	LogFilename        string        `envconfig:"LOG_FILENAME" default:""`
}

// Validate performs semantic validation on the configuration. It is exported
// so callers that override a field after Load (e.g. the -mode flag) can
// re-validate the resulting configuration.
func (c Config) Validate() error {
	return c.validate()
}

// validate performs semantic validation on the loaded configuration.
func (c Config) validate() error {
	return validation.ValidateStruct(&c,
		validation.Field(&c.SearXNGURL,
			validation.Required,
			validation.By(validateURLScheme),
			validation.By(validateURLHost),
		),
		validation.Field(&c.RateLimitGlobal, validation.By(validatePositiveInt)),
		validation.Field(&c.RateLimitPerClient, validation.By(validatePositiveInt)),
		validation.Field(&c.WriteTimeout, validation.Min(time.Duration(0))),
		validation.Field(&c.Mode, validation.By(validateMode)),
		validation.Field(&c.LogLevel, validation.By(validateLogLevel)),
		validation.Field(&c.LogFormat, validation.By(validateLogFormat)),
	)
}

func validateMode(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if s != "http" && s != "stdio" {
		return fmt.Errorf("must be either \"http\" or \"stdio\" (got %q)", s)
	}
	return nil
}

// validateLogLevel accepts a nil *string (LOG_LEVEL unset) as valid and only
// validates the value when it is non-nil.
func validateLogLevel(value interface{}) error {
	s, ok := value.(*string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if s == nil {
		return nil
	}
	if _, err := logging.ParseLevel(*s); err != nil {
		return err
	}
	return nil
}

func validateLogFormat(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if _, err := logging.ParseFormat(s); err != nil {
		return err
	}
	return nil
}

func validatePositiveInt(value interface{}) error {
	n, ok := value.(int)
	if !ok {
		return fmt.Errorf("must be an integer")
	}
	if n < 1 {
		return fmt.Errorf("must be at least 1 (got %d)", n)
	}
	return nil
}

func validateURLScheme(value interface{}) error {
	u, err := url.Parse(value.(string))
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must use http or https scheme (got %q)", u.Scheme)
	}
	return nil
}

func validateURLHost(value interface{}) error {
	u, err := url.Parse(value.(string))
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("must include a host (e.g. http://searxng:8888)")
	}
	return nil
}

// legacyMetricsAddr is the previous environment variable name for the internal
// observability listener address. It is honoured only when INTERNAL_ADDR is not
// explicitly set, so the new variable wins whenever both are present.
const legacyMetricsAddr = "PROMETHEUS_METRICS_ADDR"

// Load reads configuration from environment variables and validates it.
// Returns the parsed Config or an error.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	// Legacy shim: PROMETHEUS_METRICS_ADDR predates INTERNAL_ADDR. envconfig has
	// no aliasing support, so translate the legacy variable manually — but only
	// when INTERNAL_ADDR was not explicitly provided, letting INTERNAL_ADDR win
	// whenever both are set.
	if os.Getenv("INTERNAL_ADDR") == "" {
		if legacy := os.Getenv(legacyMetricsAddr); legacy != "" {
			cfg.InternalAddr = legacy
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}
