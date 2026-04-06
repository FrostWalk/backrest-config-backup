package config

import (
	"testing"
)

func TestLoadFromEnvRequiresCriticalValues(t *testing.T) {
	t.Setenv("CONFIG_PATH", "")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("AGE_PASSPHRASE_FILE", "")
	t.Setenv("CRON_SCHEDULE", "")
	t.Setenv("HEALTHCHECKS_URL", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestLoadFromEnvDefaultsAndNormalization(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/data/config.json")
	t.Setenv("S3_BUCKET", "bucket")
	t.Setenv("S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("AGE_PASSPHRASE_FILE", "/run/secrets/age_passphrase")
	t.Setenv("CRON_SCHEDULE", "*/5 * * * *")
	t.Setenv("HEALTHCHECKS_URL", "https://hc-ping.com/uuid")
	t.Setenv("S3_PREFIX", "/backups/")
	t.Setenv("RUN_TIMEOUT", "45s")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.S3Prefix != "backups/" {
		t.Fatalf("unexpected normalized prefix: %q", cfg.S3Prefix)
	}
	if cfg.RunTimeout.String() != "45s" {
		t.Fatalf("unexpected timeout: %s", cfg.RunTimeout.String())
	}
}

func TestLoadFromEnvRunOnceBypassesCronRequirement(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/data/config.json")
	t.Setenv("S3_BUCKET", "bucket")
	t.Setenv("S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("AGE_PASSPHRASE_FILE", "/run/secrets/age_passphrase")
	t.Setenv("CRON_SCHEDULE", "")
	t.Setenv("HEALTHCHECKS_URL", "https://hc-ping.com/uuid")
	t.Setenv("RUN_ONCE", "true")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.RunOnce {
		t.Fatalf("expected RunOnce true")
	}
}

func TestLoadFromEnvHealthchecksOptional(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/data/config.json")
	t.Setenv("S3_BUCKET", "bucket")
	t.Setenv("S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("AGE_PASSPHRASE_FILE", "/run/secrets/age_passphrase")
	t.Setenv("CRON_SCHEDULE", "*/5 * * * *")
	t.Setenv("HEALTHCHECKS_URL", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HealthchecksURL != "" {
		t.Fatalf("expected empty HEALTHCHECKS_URL, got %q", cfg.HealthchecksURL)
	}
}
