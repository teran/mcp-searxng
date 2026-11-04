package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/teran/mcp-searxng/config"
)

// newBannerLogger returns a JSON logger capturing output for banner tests.
func newBannerLogger() (*logrus.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	return logger, &buf
}

// TestLogStartupBannerB05 verifies the startup banner (B05/L06): the first log
// line follows the "Starting {appName}/{appVersion} (commit: {commit}; built at
// {date})" template, and the SearXNG URL is redacted (host visible, credentials
// hidden) via the annotation-driven helper (S02).
func TestLogStartupBannerB05(t *testing.T) {
	t.Parallel()

	logger, buf := newBannerLogger()
	// #nosec G101 -- test fixture deliberately uses fake credentials.
	logStartupBanner(logger, config.Config{
		SearXNGURL: "https://user:pass@searxng.example.com",
	})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two banner log lines, got %d", len(lines))
	}

	var firstEntry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &firstEntry); err != nil {
		t.Fatalf("first banner line is not valid JSON: %v\nraw: %s", err, lines[0])
	}
	wantPrefix := "Starting " + appName + "/" + version + " (commit: " + commit + "; built at " + date + ")"
	if msg, _ := firstEntry["msg"].(string); !strings.HasPrefix(msg, wantPrefix) {
		t.Errorf("first banner line msg %q does not follow the B05 template %q", firstEntry["msg"], wantPrefix)
	}

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "user:pass") {
		t.Errorf("banner leaked URL credentials: %s", joined)
	}
	if !strings.Contains(joined, "searxng.example.com") {
		t.Errorf("banner should keep the URL host visible: %s", joined)
	}
}

// TestLogStartupBannerRedactsTokenURL verifies a token-only URL (no password)
// is also fully stripped of its userinfo, not just its password.
func TestLogStartupBannerRedactsTokenURL(t *testing.T) {
	t.Parallel()

	logger, buf := newBannerLogger()
	// #nosec G101 -- test fixture deliberately uses a fake token.
	logStartupBanner(logger, config.Config{
		SearXNGURL: "https://sekret-token@searxng.example.com",
	})

	if strings.Contains(buf.String(), "sekret-token") {
		t.Errorf("banner leaked URL token: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "searxng.example.com") {
		t.Errorf("banner should keep the URL host visible: %s", buf.String())
	}
}
