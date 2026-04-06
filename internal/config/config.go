package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimezone   = "UTC"
	defaultS3Region   = "us-east-1"
	defaultRunTimeout = 2 * time.Minute
)

type Config struct {
	ConfigPath        string
	S3Bucket          string
	S3Prefix          string
	S3Region          string
	S3Endpoint        string
	AgePassphraseFile string
	CronSchedule      string
	Timezone          string
	HealthchecksURL   string
	RunTimeout        time.Duration
	RunOnce           bool
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		ConfigPath:        strings.TrimSpace(os.Getenv("CONFIG_PATH")),
		S3Bucket:          strings.TrimSpace(os.Getenv("S3_BUCKET")),
		S3Prefix:          normalizePrefix(os.Getenv("S3_PREFIX")),
		S3Region:          envOrDefault("AWS_REGION", defaultS3Region),
		S3Endpoint:        strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
		AgePassphraseFile: strings.TrimSpace(os.Getenv("AGE_PASSPHRASE_FILE")),
		CronSchedule:      strings.TrimSpace(os.Getenv("CRON_SCHEDULE")),
		Timezone:          envOrDefault("TZ", defaultTimezone),
		HealthchecksURL:   strings.TrimSpace(os.Getenv("HEALTHCHECKS_URL")),
		RunTimeout:        defaultRunTimeout,
	}

	if raw := strings.TrimSpace(os.Getenv("RUN_ONCE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parsing RUN_ONCE: %w", err)
		}
		cfg.RunOnce = value
	}

	if raw := strings.TrimSpace(os.Getenv("RUN_TIMEOUT")); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parsing RUN_TIMEOUT: %w", err)
		}
		if timeout <= 0 {
			return Config{}, errors.New("RUN_TIMEOUT must be greater than zero")
		}
		cfg.RunTimeout = timeout
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	switch {
	case strings.TrimSpace(c.S3Bucket) == "":
		return errors.New("S3_BUCKET is required")
	case !c.RunOnce && strings.TrimSpace(c.CronSchedule) == "":
		return errors.New("CRON_SCHEDULE is required")
	case strings.TrimSpace(c.HealthchecksURL) == "":
		return errors.New("HEALTHCHECKS_URL is required")
	case strings.TrimSpace(c.ConfigPath) == "":
		return errors.New("CONFIG_PATH is required")
	case strings.TrimSpace(c.S3Endpoint) == "":
		return errors.New("S3_ENDPOINT is required")
	case strings.TrimSpace(c.AgePassphraseFile) == "":
		return errors.New("AGE_PASSPHRASE_FILE is required")
	case strings.TrimSpace(c.Timezone) == "":
		return errors.New("TZ cannot be empty")
	default:
		return nil
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func normalizePrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/"
}
