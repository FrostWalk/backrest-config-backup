package scheduler

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type CronScheduler struct {
	cron   *cron.Cron
	parser cron.Parser
	logger *zap.Logger
}

func NewCronScheduler(location *time.Location, logger *zap.Logger) *CronScheduler {
	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	return &CronScheduler{
		cron: cron.New(
			cron.WithLocation(location),
			cron.WithParser(parser),
		),
		parser: parser,
		logger: logger,
	}
}

func (s *CronScheduler) NextRun(spec string, from time.Time) (time.Time, error) {
	schedule, err := s.parser.Parse(spec)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(from), nil
}

func (s *CronScheduler) Start(ctx context.Context, spec string, job func(context.Context) error) error {
	var running atomic.Bool

	if _, err := s.cron.AddFunc(spec, func() {
		if !running.CompareAndSwap(false, true) {
			s.logger.Warn("skipping cron run because a previous run is still active")
			return
		}
		defer running.Store(false)

		if err := job(ctx); err != nil {
			s.logger.Error("scheduled job failed", zap.Error(err))
		}
	}); err != nil {
		return err
	}

	s.cron.Start()
	<-ctx.Done()

	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	return nil
}
