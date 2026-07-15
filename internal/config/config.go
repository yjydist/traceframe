package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	defaultAddress      = "127.0.0.1:8080"
	defaultDatabasePath = "data/traceframe.db"
	defaultWebDir       = "web/dist"
	defaultLogLevel     = "info"
)

type Config struct {
	Address       string
	DatabasePath  string
	WebDir        string
	LogLevel      string
	ModelProvider string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Address:       envOrDefault("TRACEFRAME_ADDR", defaultAddress),
		DatabasePath:  envOrDefault("TRACEFRAME_DATABASE_PATH", defaultDatabasePath),
		WebDir:        envOrDefault("TRACEFRAME_WEB_DIR", defaultWebDir),
		LogLevel:      strings.ToLower(envOrDefault("TRACEFRAME_LOG_LEVEL", defaultLogLevel)),
		ModelProvider: strings.ToLower(envOrDefault("TRACEFRAME_MODEL_PROVIDER", "none")),
		OpenAIAPIKey:  strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:   envOrDefault("TRACEFRAME_OPENAI_MODEL", "gpt-5.6"),
		OpenAIBaseURL: envOrDefault("TRACEFRAME_OPENAI_BASE_URL", "https://api.openai.com/v1"),
	}

	if err := validateLoopbackAddress(cfg.Address); err != nil {
		return Config{}, err
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("TRACEFRAME_DATABASE_PATH must not be empty")
	}
	if cfg.WebDir == "" {
		return Config{}, fmt.Errorf("TRACEFRAME_WEB_DIR must not be empty")
	}
	if !validLogLevel(cfg.LogLevel) {
		return Config{}, fmt.Errorf("TRACEFRAME_LOG_LEVEL must be debug, info, warn, or error")
	}
	if cfg.ModelProvider != "none" && cfg.ModelProvider != "openai" {
		return Config{}, fmt.Errorf("TRACEFRAME_MODEL_PROVIDER must be none or openai")
	}
	if cfg.ModelProvider == "openai" && cfg.OpenAIAPIKey == "" {
		return Config{}, fmt.Errorf("OPENAI_API_KEY is required when TRACEFRAME_MODEL_PROVIDER=openai")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid TRACEFRAME_ADDR: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("TRACEFRAME_ADDR must bind to a loopback address until authentication is configured")
	}
	return nil
}

func validLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
