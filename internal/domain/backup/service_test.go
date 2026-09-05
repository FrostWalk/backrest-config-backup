package backup

import (
	"bytes"
	"context"
	"errors"
	"path"
	"strings"
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
	latest        *StoredBackup
	latestErr     error
	uploadErr     error
	cleanupErr    error
	uploaded      bool
	deleted       int
	uploadKey     string
	uploadHash    string
	keepKey       string
	downloadData  []byte
	downloadErr   error
	downloadKey   string
	downloadCalls int
	cleanupCalls  int
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
	f.cleanupCalls++
	if f.cleanupErr != nil {
		return 0, f.cleanupErr
	}
	f.deleted = 1
	f.keepKey = keepObjectKey
	return 1, nil
}

func (f *fakeStore) DownloadBackup(_ context.Context, objectKey string) ([]byte, error) {
	f.downloadCalls++
	f.downloadKey = objectKey
	return f.downloadData, f.downloadErr
}

type decryptorFunc func(context.Context, []byte) ([]byte, error)

func (f decryptorFunc) Decrypt(ctx context.Context, encrypted []byte) ([]byte, error) {
	return f(ctx, encrypted)
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

func TestServiceRunVerification(t *testing.T) {
	t.Parallel()

	data := []byte(`{"hello":"world"}`)
	const downloaded = "ciphertext downloaded from S3"
	for _, tt := range []struct {
		name          string
		enabled       bool
		downloadErr   error
		decryptErr    error
		restored      []byte
		wantErr       string
		wantDownloads int
		wantDecrypts  int
		wantCleanups  int
		wantVerified  bool
	}{
		{name: "disabled", wantCleanups: 1},
		{name: "matching hash", enabled: true, restored: data, wantDownloads: 1, wantDecrypts: 1, wantCleanups: 1, wantVerified: true},
		{name: "download fails", enabled: true, downloadErr: errors.New("download failed"), wantDownloads: 1, wantErr: "downloading backup"},
		{name: "download times out", enabled: true, downloadErr: context.DeadlineExceeded, wantDownloads: 1, wantErr: "downloading backup"},
		{name: "decryption fails", enabled: true, decryptErr: errors.New("invalid ciphertext"), wantDownloads: 1, wantDecrypts: 1, wantErr: "decrypting backup"},
		{name: "hash mismatch", enabled: true, restored: []byte(`{"hello":"different"}`), wantDownloads: 1, wantDecrypts: 1, wantErr: "hash does not match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{
				latest:       &StoredBackup{ObjectKey: "backrest/previous.json.age", Hash: "old-hash"},
				downloadData: []byte(downloaded),
				downloadErr:  tt.downloadErr,
			}
			decryptCalls := 0
			decryptor := decryptorFunc(func(_ context.Context, encrypted []byte) ([]byte, error) {
				decryptCalls++
				if !store.uploaded || store.downloadKey != store.uploadKey || store.cleanupCalls != 0 {
					t.Error("verification must download the uploaded key before cleanup")
				}
				if !bytes.Equal(encrypted, []byte(downloaded)) {
					t.Error("verification must decrypt the downloaded ciphertext")
				}
				return tt.restored, tt.decryptErr
			})
			service, err := NewService(ServiceConfig{
				ConfigSource:      fakeConfigSource{data: data},
				Encryptor:         &fakeEncryptor{encrypted: []byte("locally encrypted data")},
				Decryptor:         decryptor,
				Store:             store,
				Clock:             fakeClock{now: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)},
				Location:          time.UTC,
				KeyPrefix:         "backrest",
				VerifyAfterUpload: tt.enabled,
			})
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			result, err := service.Run(t.Context())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("run: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got %v", tt.wantErr, err)
			}
			if tt.downloadErr != nil && !errors.Is(err, tt.downloadErr) {
				t.Errorf("expected wrapped download error, got %v", err)
			}
			if store.downloadCalls != tt.wantDownloads || decryptCalls != tt.wantDecrypts || store.cleanupCalls != tt.wantCleanups {
				t.Errorf("unexpected calls: download=%d decrypt=%d cleanup=%d", store.downloadCalls, decryptCalls, store.cleanupCalls)
			}
			if result.Verified != tt.wantVerified || result.DeletedOld != (tt.wantCleanups > 0) {
				t.Errorf("unexpected verification/cleanup result: %+v", result)
			}
			if !result.Changed || result.UploadedKey != store.uploadKey || result.PreviousKey != store.latest.ObjectKey {
				t.Errorf("missing upload details: %+v", result)
			}
		})
	}
}

func TestServiceVerificationRequiresDecryptor(t *testing.T) {
	t.Parallel()

	_, err := NewService(ServiceConfig{
		ConfigSource: fakeConfigSource{}, Encryptor: &fakeEncryptor{}, Store: &fakeStore{},
		Clock: fakeClock{}, Location: time.UTC, VerifyAfterUpload: true,
	})
	if err == nil || !strings.Contains(err.Error(), "decryptor is required") {
		t.Fatalf("expected missing decryptor error, got %v", err)
	}
}

func TestServiceVerificationCancellationPreventsCleanup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	data := []byte("config")
	store := &fakeStore{}
	service, err := NewService(ServiceConfig{
		ConfigSource: fakeConfigSource{data: data}, Encryptor: &fakeEncryptor{}, Store: store,
		Clock: fakeClock{}, Location: time.UTC, VerifyAfterUpload: true,
		Decryptor: decryptorFunc(func(context.Context, []byte) ([]byte, error) {
			cancel()
			return data, nil
		}),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	result, err := service.Run(ctx)
	if !errors.Is(err, context.Canceled) || result.Verified || store.cleanupCalls != 0 {
		t.Fatalf("expected canceled verification without cleanup, got %+v, %v, cleanup=%d", result, err, store.cleanupCalls)
	}
}

func TestBackupObjectKeysAreUniqueAtSameTime(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"", "backrest/config"} {
		t.Run(prefix, func(t *testing.T) {
			s := &Service{keyPrefix: prefix}
			now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
			first, second := s.buildBackupObjectKey(now), s.buildBackupObjectKey(now)
			if first == second {
				t.Fatal("backups at the same time must have distinct keys")
			}
			if !strings.HasPrefix(first, path.Join(prefix, "config-backup-2026-09-06T12-00-00-")) || !strings.HasSuffix(first, ".json.age") {
				t.Fatalf("unexpected key: %q", first)
			}
		})
	}
}
