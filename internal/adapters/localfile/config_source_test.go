package localfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSourceReadConfigSuccess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	expected := []byte(`{"version":1}`)
	if err := os.WriteFile(configPath, expected, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	source := NewConfigSource(configPath)
	got, err := source.ReadConfig(context.Background())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(expected) {
		t.Fatalf("unexpected config content: got %q want %q", string(got), string(expected))
	}
}

func TestConfigSourceReadConfigCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	source := NewConfigSource("/tmp/non-existing.json")
	_, err := source.ReadConfig(ctx)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
}

func TestConfigSourceReadConfigMissingFile(t *testing.T) {
	t.Parallel()

	source := NewConfigSource("/tmp/non-existing.json")
	_, err := source.ReadConfig(context.Background())
	if err == nil {
		t.Fatalf("expected file read error")
	}
}
