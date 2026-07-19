package main

import (
	"strings"
	"testing"
)

func TestLoadConfigFromEnvRunMigrationsDefaultFalse(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/resource_test")
	t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")
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
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/resource_test")
	t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")

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
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/resource_test")
	t.Setenv("AUTH_STATIC_TOKENS", "tok:tech-1:technician")
	t.Setenv("RUN_MIGRATIONS", "yes")

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatal("loadConfigFromEnv() error = nil, want invalid RUN_MIGRATIONS error")
	}
	if !strings.Contains(err.Error(), "RUN_MIGRATIONS") {
		t.Fatalf("error = %q, want mention RUN_MIGRATIONS", err.Error())
	}
}
