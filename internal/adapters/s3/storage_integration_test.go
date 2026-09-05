//go:build integration

package s3adapter

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	ageadapter "github.com/FrostWalk/backrest-config-backup/internal/adapters/age"
	"github.com/FrostWalk/backrest-config-backup/internal/adapters/localfile"
	"github.com/FrostWalk/backrest-config-backup/internal/domain/backup"
)

func TestStorageBackupRestoreIntegration(t *testing.T) {
	for _, name := range []string{
		"INTEGRATION_S3_BUCKET", "INTEGRATION_AWS_REGION",
		"INTEGRATION_S3_ACCESS_KEY_ID", "INTEGRATION_S3_SECRET_ACCESS_KEY",
	} {
		if os.Getenv(name) == "" {
			t.Skipf("set %s to run the S3 backup/restore integration test", name)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	client, err := NewClient(ctx, ClientConfig{
		Region:          os.Getenv("INTEGRATION_AWS_REGION"),
		Endpoint:        os.Getenv("INTEGRATION_S3_ENDPOINT"),
		AccessKeyID:     os.Getenv("INTEGRATION_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("INTEGRATION_S3_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("INTEGRATION_S3_SESSION_TOKEN"),
	})
	if err != nil {
		t.Fatalf("create s3 client: %v", err)
	}

	// Every test owns a fresh prefix; cleanup can only touch this test's objects.
	prefix := "backrest-config-backup-test/" + rand.Text()
	store := NewStorage(client, os.Getenv("INTEGRATION_S3_BUCKET"), prefix)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := store.CleanupBackups(cleanupCtx, ""); err != nil {
			t.Errorf("clean up integration prefix %q: %v", prefix, err)
		}
	})

	const plaintext = `{"fixture":"backup restore integration test"}`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	secretPath := filepath.Join(dir, "passphrase")
	for path, data := range map[string]string{configPath: plaintext, secretPath: rand.Text()} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	encryptor, err := ageadapter.NewEncryptor(secretPath)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	service, err := backup.NewService(backup.ServiceConfig{
		ConfigSource:      localfile.NewConfigSource(configPath),
		Encryptor:         encryptor,
		Decryptor:         encryptor,
		Store:             store,
		Clock:             backup.NewRealClock(),
		Location:          time.UTC,
		KeyPrefix:         prefix,
		VerifyAfterUpload: true,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	first, err := service.Run(ctx)
	if err != nil || !first.Verified || first.DeletedOld {
		t.Fatalf("initial verified backup: %+v, %v", first, err)
	}
	second, err := service.Run(ctx)
	if err != nil || !second.Verified || !second.DeletedOld || second.UploadedKey == first.UploadedKey {
		t.Fatalf("replacement verified backup: %+v, %v", second, err)
	}
	objects, err := store.listAllObjects(ctx)
	if err != nil || len(objects) != 1 {
		t.Fatalf("expected one remaining backup, got %d, %v", len(objects), err)
	}
	latest, err := store.GetLatestBackup(ctx)
	if err != nil || latest == nil || latest.ObjectKey != second.UploadedKey || latest.Hash != second.CurrentHash {
		t.Fatalf("latest backup metadata: %+v, %v", latest, err)
	}

	// A fresh decryptor checks recovery using only the persisted passphrase and object.
	decryptor, err := ageadapter.NewEncryptor(secretPath)
	if err != nil {
		t.Fatalf("new restore decryptor: %v", err)
	}
	ciphertext, err := store.DownloadBackup(ctx, latest.ObjectKey)
	if err != nil {
		t.Fatalf("download backup: %v", err)
	}
	restored, err := decryptor.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	expectedHash := sha512.Sum512([]byte(plaintext))
	restoredHash := sha512.Sum512(restored)
	if restoredHash != expectedHash || hex.EncodeToString(restoredHash[:]) != latest.Hash {
		t.Fatal("restored configuration hash differs from the original snapshot or metadata")
	}
}
