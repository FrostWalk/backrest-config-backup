package scheduler

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCronSchedulerStartInvalidSpec(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	s := NewCronScheduler(location, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.Start(ctx, "not-a-valid-spec", func(context.Context) error { return nil })
	if err == nil {
		t.Fatalf("expected invalid cron spec error")
	}
}

func TestCronSchedulerRunsJob(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	s := NewCronScheduler(location, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	startErr := make(chan error, 1)

	go func() {
		startErr <- s.Start(ctx, "@every 1s", func(context.Context) error {
			select {
			case <-done:
			default:
				close(done)
			}
			cancel()
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("scheduled job did not run in time")
	}

	if err := <-startErr; err != nil {
		t.Fatalf("scheduler start returned error: %v", err)
	}
}

func TestCronSchedulerNextRun(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	s := NewCronScheduler(location, zap.NewNop())
	from := time.Date(2026, 4, 6, 10, 1, 10, 0, location)
	next, err := s.NextRun("*/5 * * * *", from)
	if err != nil {
		t.Fatalf("next run: %v", err)
	}

	expected := time.Date(2026, 4, 6, 10, 5, 0, 0, location)
	if !next.Equal(expected) {
		t.Fatalf("next run mismatch: got %s want %s", next, expected)
	}
}
