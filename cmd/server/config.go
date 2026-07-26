package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type serverConfig struct {
	AppEnv          string
	DatabaseURL     string
	StaticTokens    string
	RunMigrations   bool
	HTTPAddr        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func loadConfigFromEnv() (serverConfig, error) {
	const (
		defaultHTTPAddr        = ":8080"
		defaultReadTimeout     = 15 * time.Second
		defaultWriteTimeout    = 15 * time.Second
		defaultIdleTimeout     = 60 * time.Second
		defaultShutdownTimeout = 10 * time.Second
	)

	cfg := serverConfig{
		AppEnv:       strings.TrimSpace(os.Getenv("APP_ENV")),
		DatabaseURL:  strings.TrimSpace(os.Getenv("DATABASE_URL")),
		StaticTokens: strings.TrimSpace(os.Getenv("AUTH_STATIC_TOKENS")),
		HTTPAddr:     strings.TrimSpace(os.Getenv("HTTP_ADDR")),
	}
	if cfg.AppEnv == "" {
		return serverConfig{}, fmt.Errorf("APP_ENV is required")
	}
	switch cfg.AppEnv {
	case "dev", "staging", "prod":
	default:
		return serverConfig{}, fmt.Errorf("APP_ENV must be one of: dev, staging, prod")
	}
	if cfg.DatabaseURL == "" {
		return serverConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AppEnv != "dev" && cfg.StaticTokens != "" {
		return serverConfig{}, fmt.Errorf("AUTH_STATIC_TOKENS is allowed only when APP_ENV=dev")
	}
	if cfg.StaticTokens == "" {
		return serverConfig{}, fmt.Errorf("AUTH_STATIC_TOKENS is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}

	runMigrations, err := parseRunMigrationsEnv("RUN_MIGRATIONS")
	if err != nil {
		return serverConfig{}, err
	}
	cfg.RunMigrations = runMigrations

	readTimeout, err := parseDurationEnv("HTTP_READ_TIMEOUT", defaultReadTimeout)
	if err != nil {
		return serverConfig{}, err
	}
	writeTimeout, err := parseDurationEnv("HTTP_WRITE_TIMEOUT", defaultWriteTimeout)
	if err != nil {
		return serverConfig{}, err
	}
	idleTimeout, err := parseDurationEnv("HTTP_IDLE_TIMEOUT", defaultIdleTimeout)
	if err != nil {
		return serverConfig{}, err
	}
	shutdownTimeout, err := parseDurationEnv("HTTP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return serverConfig{}, err
	}

	cfg.ReadTimeout = readTimeout
	cfg.WriteTimeout = writeTimeout
	cfg.IdleTimeout = idleTimeout
	cfg.ShutdownTimeout = shutdownTimeout

	return cfg, nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s parse duration %q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}

	return d, nil
}

func parseRunMigrationsEnv(key string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, nil
	}

	switch strings.ToLower(raw) {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be one of: true, false, 1, 0", key)
	}
}
