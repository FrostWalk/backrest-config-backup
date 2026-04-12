package healthcheck

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const defaultPingTimeout = 10 * time.Second

type pinger interface {
	PingSuccess(ctx context.Context) error
	PingFailure(ctx context.Context, reason string) error
}

type Reporter struct {
	enabled  bool
	notifier pinger
	logger   *zap.Logger
	timeout  time.Duration
}

func NewReporter(client *http.Client, baseURL string, logger *zap.Logger) *Reporter {
	enabled := strings.TrimSpace(baseURL) != ""
	reporter := &Reporter{
		enabled: enabled,
		logger:  logger,
		timeout: defaultPingTimeout,
	}
	if !enabled {
		if logger != nil {
			logger.Info("healthchecks disabled; HEALTHCHECKS_URL not set")
		}
		return reporter
	}

	reporter.notifier = NewNotifier(client, baseURL)
	return reporter
}

func (r *Reporter) NotifyFailure(ctx context.Context, reason string) {
	if !r.enabled {
		return
	}

	pingCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := r.notifier.PingFailure(pingCtx, reason); err != nil && r.logger != nil {
		r.logger.Error("failure ping failed", zap.Error(err))
	}
}

func (r *Reporter) NotifySuccess(ctx context.Context) error {
	if !r.enabled {
		return nil
	}

	pingCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := r.notifier.PingSuccess(pingCtx); err != nil {
		if r.logger != nil {
			r.logger.Error(
				"success ping failed",
				zap.Error(err),
			)
		}
		return err
	}

	return nil
}
