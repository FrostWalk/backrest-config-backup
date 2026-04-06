package observability

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()

	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	if logger == nil {
		t.Fatalf("expected non-nil logger")
	}
}

func TestSyncLoggerNilSafe(t *testing.T) {
	t.Parallel()

	if err := SyncLogger(nil); err != nil {
		t.Fatalf("expected nil error for nil logger, got %v", err)
	}
}

func TestSyncLoggerNop(t *testing.T) {
	t.Parallel()

	if err := SyncLogger(zap.NewNop()); err != nil {
		t.Fatalf("expected nil error for nop logger, got %v", err)
	}
}
