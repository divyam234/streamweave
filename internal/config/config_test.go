package config

import (
	"strings"
	"testing"
)

func TestProductionRequiresSecurityConfiguration(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("MASTER_KEY", "")
	t.Setenv("ADMIN_TOKEN", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestProductionConfiguration(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MASTER_KEY", strings.Repeat("a", 64))
	t.Setenv("ADMIN_TOKEN", strings.Repeat("b", 48))
	t.Setenv("ALLOW_PRIVATE_PROVIDER_ENDPOINTS", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Production {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestProductionRejectsPrivateProviderMode(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MASTER_KEY", strings.Repeat("a", 64))
	t.Setenv("ADMIN_TOKEN", strings.Repeat("b", 48))
	t.Setenv("ALLOW_PRIVATE_PROVIDER_ENDPOINTS", "true")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ALLOW_PRIVATE_PROVIDER_ENDPOINTS") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestProductionRejectsInsecureResolverOverride(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MASTER_KEY", strings.Repeat("a", 64))
	t.Setenv("ADMIN_TOKEN", strings.Repeat("b", 48))
	t.Setenv("ALLOW_PRIVATE_PROVIDER_ENDPOINTS", "false")
	t.Setenv("REALDEBRID_BASE_URL", "http://api.example.com")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "REALDEBRID_BASE_URL") {
		t.Fatalf("Load error = %v", err)
	}
}
