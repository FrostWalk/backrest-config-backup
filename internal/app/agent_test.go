package app

import (
	"context"
	"errors"
	"testing"

	"github.com/FrostWalk/backrest-config-backup/internal/domain/backup"
	"go.uber.org/zap"
)

type fakeNotifier struct {
	successCalls int
	successErr   error
}

func (f *fakeNotifier) PingSuccess(context.Context) error {
	f.successCalls++
	return f.successErr
}

func (f *fakeNotifier) PingFailure(context.Context, string) error {
	return nil
}

func TestSendSuccessPing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		enabled    bool
		result     backup.RunResult
		successErr error
		wantCalls  int
		wantError  bool
	}{
		{
			name:      "disabled does not ping",
			enabled:   false,
			result:    backup.RunResult{SkippedEqual: true},
			wantCalls: 0,
			wantError: false,
		},
		{
			name:      "changed backup sends success ping",
			enabled:   true,
			result:    backup.RunResult{Changed: true},
			wantCalls: 1,
			wantError: false,
		},
		{
			name:      "unchanged backup sends success ping",
			enabled:   true,
			result:    backup.RunResult{SkippedEqual: true},
			wantCalls: 1,
			wantError: false,
		},
		{
			name:       "ping error is returned",
			enabled:    true,
			result:     backup.RunResult{SkippedEqual: true},
			successErr: errors.New("boom"),
			wantCalls:  1,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notifier := &fakeNotifier{successErr: tt.successErr}
			err := sendSuccessPing(context.Background(), notifier, tt.enabled, tt.result, zap.NewNop())
			if tt.wantError && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if notifier.successCalls != tt.wantCalls {
				t.Fatalf("success ping calls mismatch: got %d want %d", notifier.successCalls, tt.wantCalls)
			}
		})
	}
}
