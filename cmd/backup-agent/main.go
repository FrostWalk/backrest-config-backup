package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	ageadapter "github.com/FrostWalk/backrest-config-backup/internal/adapters/age"
	"github.com/FrostWalk/backrest-config-backup/internal/adapters/healthchecks"
	"github.com/FrostWalk/backrest-config-backup/internal/adapters/localfile"
	"github.com/FrostWalk/backrest-config-backup/internal/adapters/s3"
	"github.com/FrostWalk/backrest-config-backup/internal/adapters/scheduler"
	"github.com/FrostWalk/backrest-config-backup/internal/config"
	"github.com/FrostWalk/backrest-config-backup/internal/domain/backup"
	"github.com/FrostWalk/backrest-config-backup/internal/observability"
	"github.com/FrostWalk/backrest-config-backup/internal/version"
	"go.uber.org/zap"
)

func main() {
	if printVersion() {
		return
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func printVersion() bool {
	for _, a := range os.Args[1:] {
		switch a {
		case "-version", "--version", "-v":
			fmt.Printf("backup-agent %s\n  commit: %s\n  built:  %s\n", version.Version, version.Revision, version.BuildDate)
			return true
		}
	}
	return false
}

func run() error {
	logger, err := observability.NewLogger()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer func() {
		_ = observability.SyncLogger(logger)
	}()

	logger.Info(
		"backup-agent",
		zap.String("version", version.Version),
		zap.String("revision", version.Revision),
		zap.String("build_date", version.BuildDate),
	)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("loading config from env: %w", err)
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return fmt.Errorf("loading timezone %q: %w", cfg.Timezone, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	client, err := s3adapter.NewClient(ctx, s3adapter.ClientConfig{
		Region:   cfg.S3Region,
		Endpoint: cfg.S3Endpoint,
	})
	if err != nil {
		return fmt.Errorf("creating s3 client: %w", err)
	}

	store := s3adapter.NewStorage(client, cfg.S3Bucket, cfg.S3Prefix)
	encryptor, err := ageadapter.NewEncryptor(cfg.AgePassphraseFile)
	if err != nil {
		return fmt.Errorf("creating age encryptor: %w", err)
	}

	service, err := backup.NewService(backup.ServiceConfig{
		ConfigSource: localfile.NewConfigSource(cfg.ConfigPath),
		Encryptor:    encryptor,
		Store:        store,
		Clock:        backup.NewRealClock(),
		Location:     location,
		KeyPrefix:    cfg.S3Prefix,
	})
	if err != nil {
		return fmt.Errorf("creating backup service: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	notifier := healthchecks.NewNotifier(httpClient, cfg.HealthchecksURL)
	cronScheduler := scheduler.NewCronScheduler(location, logger)

	job := func(runParent context.Context) error {
		start := time.Now()
		runCtx, cancelRun := context.WithTimeout(runParent, cfg.RunTimeout)
		defer cancelRun()
		nextRun, nextRunErr := cronScheduler.NextRun(cfg.CronSchedule, time.Now().In(location))

		result, err := service.Run(runCtx)
		duration := time.Since(start)
		if err != nil {
			fields := []zap.Field{
				zap.Error(err),
				zap.Duration("duration", duration),
			}
			if nextRunErr == nil {
				fields = append(fields, zap.Time("next_backup_at", nextRun))
			}
			logger.Error("backup run failed", fields...)
			pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelPing()
			if pingErr := notifier.PingFailure(pingCtx, err.Error()); pingErr != nil {
				logger.Error("failure ping failed", zap.Error(pingErr))
			}
			return err
		}

		fields := []zap.Field{
			zap.Bool("changed", result.Changed),
			zap.Bool("skipped_equal", result.SkippedEqual),
			zap.String("uploaded_key", result.UploadedKey),
			zap.String("previous_key", result.PreviousKey),
			zap.Bool("deleted_old", result.DeletedOld),
			zap.Duration("duration", duration),
		}
		if nextRunErr == nil {
			fields = append(fields, zap.Time("next_backup_at", nextRun))
		}
		logger.Info("backup run succeeded", fields...)

		pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelPing()
		if pingErr := notifier.PingSuccess(pingCtx); pingErr != nil {
			logger.Error("success ping failed", zap.Error(pingErr))
			return pingErr
		}
		return nil
	}

	if cfg.RunOnce {
		logger.Info("run once enabled; running backup immediately and exiting")
		return job(ctx)
	}

	nextRun, err := cronScheduler.NextRun(cfg.CronSchedule, time.Now().In(location))
	if err != nil {
		return fmt.Errorf("computing next backup run: %w", err)
	}
	logger.Info(
		"backup scheduler started",
		zap.String("schedule", cfg.CronSchedule),
		zap.String("timezone", cfg.Timezone),
		zap.Time("next_backup_at", nextRun),
	)

	if err := cronScheduler.Start(ctx, cfg.CronSchedule, job); err != nil {
		return fmt.Errorf("starting cron scheduler: %w", err)
	}

	logger.Info("backup scheduler stopped")
	return nil
}
