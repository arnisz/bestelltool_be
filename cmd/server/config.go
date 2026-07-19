package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type serverConfig struct {
	DatabaseURL     string
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
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(os.Getenv("HTTP_ADDR")),
	}
	if cfg.DatabaseURL == "" {
		return serverConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}

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
