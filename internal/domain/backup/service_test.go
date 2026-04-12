package backup

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeConfigSource struct {
	data []byte
	err  error
}

func (f fakeConfigSource) ReadConfig(context.Context) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

type fakeEncryptor struct {
	encrypted []byte
	err       error
	calls     int
}

func (f *fakeEncryptor) Encrypt(context.Context, []byte) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.encrypted, nil
}

type fakeStore struct {
	latest     *StoredBackup
	latestErr  error
	uploadErr  error
	cleanupErr error
	uploaded   bool
	deleted    int
	uploadKey  string
	uploadHash string
	keepKey    string
}

func (f *fakeStore) GetLatestBackup(context.Context) (*StoredBackup, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	return f.latest, nil
}

func (f *fakeStore) UploadBackup(_ context.Context, objectKey string, _ []byte, configHash string) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.uploaded = true
	f.uploadKey = objectKey
	f.uploadHash = configHash
	return nil
}

func (f *fakeStore) CleanupBackups(_ context.Context, keepObjectKey string) (int, error) {
	if f.cleanupErr != nil {
		return 0, f.cleanupErr
	}
	f.deleted = 1
	f.keepKey = keepObjectKey
	return 1, nil
}

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time {
	return f.now
}

func TestServiceRunUnchangedHashStillUploadsAndDeletesPrevious(t *testing.T) {
	t.Parallel()

	data := []byte(`{"hello":"world"}`)
	hash := hashBytes(data)
	encryptor := &fakeEncryptor{encrypted: []byte("encrypted")}
	store := &fakeStore{latest: &StoredBackup{ObjectKey: "old-key", Hash: hash}}
	location, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	service, err := NewService(ServiceConfig{
		ConfigSource: fakeConfigSource{data: data},
		Encryptor:    encryptor,
		Store:        store,
		Clock:        fakeClock{now: time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)},
		Location:     location,
		KeyPrefix:    "backrest",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed true")
	}
	if !store.uploaded {
		t.Fatalf("expected upload called")
	}
	if store.deleted == 0 {
		t.Fatalf("expected cleanup called")
	}
	if encryptor.calls != 1 {
		t.Fatalf("expected encrypt called once, got %d", encryptor.calls)
	}
}

func TestServiceRunChangedHashUploadsAndDeletesPrevious(t *testing.T) {
	t.Parallel()

	data := []byte(`{"hello":"world"}`)
	encryptor := &fakeEncryptor{encrypted: []byte("encrypted")}
	store := &fakeStore{latest: &StoredBackup{ObjectKey: "old-key", Hash: "different"}}
	location, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	service, err := NewService(ServiceConfig{
		ConfigSource: fakeConfigSource{data: data},
		Encryptor:    encryptor,
		Store:        store,
		Clock:        fakeClock{now: time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)},
		Location:     location,
		KeyPrefix:    "backrest",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed true")
	}
	if !store.uploaded {
		t.Fatalf("expected upload called")
	}
	if store.deleted == 0 {
		t.Fatalf("expected cleanup called")
	}
	if result.UploadedKey == "" {
		t.Fatalf("expected uploaded key")
	}
	if store.keepKey == "" {
		t.Fatalf("expected keep key set")
	}
}

func TestServiceRunUploadFailureDoesNotDelete(t *testing.T) {
	t.Parallel()

	encryptor := &fakeEncryptor{encrypted: []byte("encrypted")}
	store := &fakeStore{
		latest:    &StoredBackup{ObjectKey: "old-key", Hash: "different"},
		uploadErr: errors.New("upload failed"),
	}
	location, _ := time.LoadLocation("UTC")
	service, err := NewService(ServiceConfig{
		ConfigSource: fakeConfigSource{data: []byte("data")},
		Encryptor:    encryptor,
		Store:        store,
		Clock:        fakeClock{now: time.Now()},
		Location:     location,
		KeyPrefix:    "backrest",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if store.deleted > 0 {
		t.Fatalf("expected cleanup not called")
	}
}
