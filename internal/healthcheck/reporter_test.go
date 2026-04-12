package healthcheck

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

type fakePinger struct {
	successCalls int
	successErr   error
	failureCalls int
	failureErr   error
	lastReason   string
}

func (f *fakePinger) PingSuccess(context.Context) error {
	f.successCalls++
	return f.successErr
}

func (f *fakePinger) PingFailure(_ context.Context, reason string) error {
	f.failureCalls++
	f.lastReason = reason
	return f.failureErr
}

func TestReporterNotifySuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reporter  Reporter
		wantCalls int
		wantErr   bool
	}{
		{
			name: "disabled does not ping",
			reporter: Reporter{
				enabled:  false,
				notifier: &fakePinger{},
				logger:   zap.NewNop(),
			},
			wantCalls: 0,
			wantErr:   false,
		},
		{
			name: "enabled sends success ping",
			reporter: Reporter{
				enabled:  true,
				notifier: &fakePinger{},
				logger:   zap.NewNop(),
			},
			wantCalls: 1,
			wantErr:   false,
		},
		{
			name: "ping error is returned",
			reporter: Reporter{
				enabled: true,
				notifier: &fakePinger{
					successErr: errors.New("boom"),
				},
				logger: zap.NewNop(),
			},
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pinger, ok := tt.reporter.notifier.(*fakePinger)
			if !ok {
				t.Fatalf("reporter notifier is not fakePinger")
			}

			err := tt.reporter.NotifySuccess(context.Background())
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if pinger.successCalls != tt.wantCalls {
				t.Fatalf("success ping calls mismatch: got %d want %d", pinger.successCalls, tt.wantCalls)
			}
		})
	}
}

func TestReporterNotifyFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reporter  Reporter
		reason    string
		wantCalls int
	}{
		{
			name: "disabled does not ping",
			reporter: Reporter{
				enabled:  false,
				notifier: &fakePinger{},
				logger:   zap.NewNop(),
			},
			reason:    "error",
			wantCalls: 0,
		},
		{
			name: "enabled sends failure ping",
			reporter: Reporter{
				enabled:  true,
				notifier: &fakePinger{},
				logger:   zap.NewNop(),
			},
			reason:    "error",
			wantCalls: 1,
		},
		{
			name: "ping error is swallowed",
			reporter: Reporter{
				enabled: true,
				notifier: &fakePinger{
					failureErr: errors.New("boom"),
				},
				logger: zap.NewNop(),
			},
			reason:    "error",
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pinger, ok := tt.reporter.notifier.(*fakePinger)
			if !ok {
				t.Fatalf("reporter notifier is not fakePinger")
			}

			tt.reporter.NotifyFailure(context.Background(), tt.reason)
			if pinger.failureCalls != tt.wantCalls {
				t.Fatalf("failure ping calls mismatch: got %d want %d", pinger.failureCalls, tt.wantCalls)
			}
			if pinger.lastReason != "" && pinger.lastReason != tt.reason {
				t.Fatalf("failure reason mismatch: got %q want %q", pinger.lastReason, tt.reason)
			}
		})
	}
}
