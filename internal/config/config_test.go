package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TRACEFRAME_ADDR", "")
	t.Setenv("TRACEFRAME_DATABASE_PATH", "test.db")
	t.Setenv("TRACEFRAME_WEB_DIR", "web/dist")
	t.Setenv("TRACEFRAME_LOG_LEVEL", "info")
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "none")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected an error for an explicitly empty address")
	}
}

func TestLoadAcceptsLoopback(t *testing.T) {
	t.Setenv("TRACEFRAME_ADDR", "[::1]:9090")
	t.Setenv("TRACEFRAME_DATABASE_PATH", ":memory:")
	t.Setenv("TRACEFRAME_WEB_DIR", "web/dist")
	t.Setenv("TRACEFRAME_LOG_LEVEL", "DEBUG")
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "none")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadRejectsNonLoopback(t *testing.T) {
	t.Setenv("TRACEFRAME_ADDR", "0.0.0.0:8080")
	t.Setenv("TRACEFRAME_DATABASE_PATH", ":memory:")
	t.Setenv("TRACEFRAME_WEB_DIR", "web/dist")
	t.Setenv("TRACEFRAME_LOG_LEVEL", "info")
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "none")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for a non-loopback address")
	}
}

func TestLoadRejectsOpenAIWithoutAPIKey(t *testing.T) {
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY is required") {
		t.Fatalf("Load() error = %v, want missing API key error", err)
	}
}

func TestLoadRejectsUnsupportedProviderWithoutExposingAPIKey(t *testing.T) {
	const secret = "sk-test-secret-must-not-leak"
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "unsupported")
	t.Setenv("OPENAI_API_KEY", secret)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TRACEFRAME_MODEL_PROVIDER must be none or openai") {
		t.Fatalf("Load() error = %v, want unsupported provider error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Load() error exposed API key: %v", err)
	}
}

func TestLoadRepositoryLimits(t *testing.T) {
	t.Setenv("TRACEFRAME_MODEL_PROVIDER", "none")
	t.Setenv("TRACEFRAME_REPOSITORY_MAX_FILE_BYTES", "4096")
	t.Setenv("TRACEFRAME_REPOSITORY_MAX_RESULTS", "0")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TRACEFRAME_REPOSITORY_MAX_RESULTS") {
		t.Fatalf("Load() error = %v, want invalid repository limit", err)
	}
	t.Setenv("TRACEFRAME_REPOSITORY_MAX_RESULTS", "25")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RepositoryMaxFileBytes != 4096 || cfg.RepositoryMaxResults != 25 {
		t.Fatalf("repository limits = %d/%d", cfg.RepositoryMaxFileBytes, cfg.RepositoryMaxResults)
	}
}
