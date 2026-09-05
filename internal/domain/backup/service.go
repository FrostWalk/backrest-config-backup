package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

type Service struct {
	configSource      ConfigSource
	encryptor         Encryptor
	decryptor         Decryptor
	store             Store
	clock             Clock
	location          *time.Location
	keyPrefix         string
	verifyAfterUpload bool
}

type ServiceConfig struct {
	ConfigSource      ConfigSource
	Encryptor         Encryptor
	Decryptor         Decryptor
	Store             Store
	Clock             Clock
	Location          *time.Location
	KeyPrefix         string
	VerifyAfterUpload bool
}

type RunResult struct {
	Changed     bool
	CurrentHash string
	UploadedKey string
	DeletedOld  bool
	PreviousKey string
	Verified    bool
}

func NewService(cfg ServiceConfig) (*Service, error) {
	switch {
	case cfg.ConfigSource == nil:
		return nil, errors.New("config source is required")
	case cfg.Encryptor == nil:
		return nil, errors.New("encryptor is required")
	case cfg.Store == nil:
		return nil, errors.New("backup store is required")
	case cfg.Clock == nil:
		return nil, errors.New("clock is required")
	case cfg.Location == nil:
		return nil, errors.New("location is required")
	case cfg.VerifyAfterUpload && cfg.Decryptor == nil:
		return nil, errors.New("decryptor is required when verification is enabled")
	}

	return &Service{
		configSource:      cfg.ConfigSource,
		encryptor:         cfg.Encryptor,
		decryptor:         cfg.Decryptor,
		store:             cfg.Store,
		clock:             cfg.Clock,
		location:          cfg.Location,
		keyPrefix:         strings.Trim(strings.TrimSpace(cfg.KeyPrefix), "/"),
		verifyAfterUpload: cfg.VerifyAfterUpload,
	}, nil
}

func (s *Service) Run(ctx context.Context) (RunResult, error) {
	var result RunResult

	plaintext, err := s.configSource.ReadConfig(ctx)
	if err != nil {
		return result, fmt.Errorf("reading configuration: %w", err)
	}
	currentHash := hashBytes(plaintext)
	result.CurrentHash = currentHash

	previous, err := s.store.GetLatestBackup(ctx)
	if err != nil {
		return result, fmt.Errorf("getting latest backup metadata: %w", err)
	}
	if previous != nil {
		result.PreviousKey = previous.ObjectKey
	}

	encrypted, err := s.encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		return result, fmt.Errorf("encrypting configuration: %w", err)
	}

	key := s.buildBackupObjectKey(s.clock.Now().In(s.location))
	if err := s.store.UploadBackup(ctx, key, encrypted, currentHash); err != nil {
		return result, fmt.Errorf("uploading encrypted backup: %w", err)
	}

	result.Changed = true
	result.UploadedKey = key
	if s.verifyAfterUpload {
		if err := s.verifyBackup(ctx, key, currentHash); err != nil {
			return result, fmt.Errorf("verifying uploaded backup %q: %w", key, err)
		}
		result.Verified = true
	}

	// Keep previous backups until upload and any requested verification succeed.
	deletedCount, err := s.store.CleanupBackups(ctx, key)
	if err != nil {
		return result, fmt.Errorf("cleaning up old backups while keeping %q: %w", key, err)
	}
	result.DeletedOld = deletedCount > 0

	return result, nil
}

func (s *Service) verifyBackup(ctx context.Context, key, expectedHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	encrypted, err := s.store.DownloadBackup(ctx, key)
	if err != nil {
		return fmt.Errorf("downloading backup: %w", err)
	}
	restored, err := s.decryptor.Decrypt(ctx, encrypted)
	if err != nil {
		return fmt.Errorf("decrypting backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hashBytes(restored) != expectedHash {
		return errors.New("restored configuration hash does not match the original snapshot")
	}
	return nil
}

func (s *Service) buildBackupObjectKey(now time.Time) string {
	filename := "config-backup-" + now.Format("2006-01-02T15-04-05") + "-" + rand.Text() + ".json.age"
	if s.keyPrefix == "" {
		return filename
	}
	return path.Join(s.keyPrefix, filename)
}

func hashBytes(data []byte) string {
	sum := sha512.Sum512(data)
	return hex.EncodeToString(sum[:])
}

type realClock struct{}

func NewRealClock() Clock {
	return realClock{}
}

func (realClock) Now() time.Time {
	return time.Now()
}
