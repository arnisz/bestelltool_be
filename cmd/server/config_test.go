package main

import (
	"strings"
	"testing"
)

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/resource_test")
	t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
}

func TestLoadConfigFromEnvRunMigrationsDefaultFalse(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("RUN_MIGRATIONS", "")

	cfg, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv() error = %v", err)
	}
	if cfg.RunMigrations {
		t.Fatalf("cfg.RunMigrations = %t, want false", cfg.RunMigrations)
	}
}

func TestLoadConfigFromEnvRunMigrationsEnabledValues(t *testing.T) {
	setBaseEnv(t)

	for _, raw := range []string{"true", "1", "TRUE"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("RUN_MIGRATIONS", raw)

			cfg, err := loadConfigFromEnv()
			if err != nil {
				t.Fatalf("loadConfigFromEnv() error = %v", err)
			}
			if !cfg.RunMigrations {
				t.Fatalf("cfg.RunMigrations = %t, want true", cfg.RunMigrations)
			}
		})
	}
}

func TestLoadConfigFromEnvRunMigrationsRejectsInvalidValue(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("RUN_MIGRATIONS", "yes")

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatal("loadConfigFromEnv() error = nil, want invalid RUN_MIGRATIONS error")
	}
	if !strings.Contains(err.Error(), "RUN_MIGRATIONS") {
		t.Fatalf("error = %q, want mention RUN_MIGRATIONS", err.Error())
	}
}

func TestLoadConfigFromEnvRequiresAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/resource_test")
	t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatal("loadConfigFromEnv() error = nil, want missing APP_ENV error")
	}
	if !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("error = %q, want mention APP_ENV", err.Error())
	}
}

func TestLoadConfigFromEnvRejectsInvalidAppEnvValue(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("APP_ENV", "qa")

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatal("loadConfigFromEnv() error = nil, want invalid APP_ENV error")
	}
	if !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("error = %q, want mention APP_ENV", err.Error())
	}
}

func TestLoadConfigFromEnvSEC26RejectsStaticTokensOutsideDev(t *testing.T) {
	for _, appEnv := range []string{"staging", "prod"} {
		t.Run(appEnv, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("APP_ENV", appEnv)
			t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")

			_, err := loadConfigFromEnv()
			if err == nil {
				t.Fatal("loadConfigFromEnv() error = nil, want SEC-26 startup error")
			}
			if !strings.Contains(err.Error(), "AUTH_STATIC_TOKENS") {
				t.Fatalf("error = %q, want mention AUTH_STATIC_TOKENS", err.Error())
			}
		})
	}
}

func TestLoadConfigFromEnvRequiresValidEncryptionKey(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("ENCRYPTION_KEY", "not-base64")

	_, err := loadConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "ENCRYPTION_KEY") {
		t.Fatalf("loadConfigFromEnv() error = %v, want ENCRYPTION_KEY validation", err)
	}
}
