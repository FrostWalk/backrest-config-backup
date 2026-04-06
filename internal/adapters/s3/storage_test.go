package s3adapter

import "testing"

func TestStorageFullPrefix(t *testing.T) {
	t.Parallel()

	storage := NewStorage(nil, "bucket", "/backrest/config/")
	if got, want := storage.fullPrefix(), "backrest/config/"; got != want {
		t.Fatalf("fullPrefix mismatch: got %q want %q", got, want)
	}
}

func TestStorageFullPrefixEmpty(t *testing.T) {
	t.Parallel()

	storage := NewStorage(nil, "bucket", "")
	if got := storage.fullPrefix(); got != "" {
		t.Fatalf("expected empty prefix, got %q", got)
	}
}
