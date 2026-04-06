package observability

import (
	"errors"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()

	// Use a human-readable timestamp in the process local timezone (driven by TZ).
	cfg.EncoderConfig.EncodeTime = func(t time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(t.In(time.Local).Format(time.RFC3339))
	}
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	return cfg.Build()
}

func SyncLogger(logger *zap.Logger) error {
	if logger == nil {
		return nil
	}

	err := logger.Sync()
	if err == nil {
		return nil
	}

	// Ignore known non-fatal sync errors for stdio-backed outputs.
	if errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}
