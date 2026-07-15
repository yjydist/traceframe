package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TRACEFRAME_ADDR", "")
	t.Setenv("TRACEFRAME_DATABASE_PATH", "test.db")
	t.Setenv("TRACEFRAME_WEB_DIR", "web/dist")
	t.Setenv("TRACEFRAME_LOG_LEVEL", "info")

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

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for a non-loopback address")
	}
}
