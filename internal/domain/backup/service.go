package backup

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

type Service struct {
	configSource ConfigSource
	encryptor    Encryptor
	store        Store
	clock        Clock
	location     *time.Location
	keyPrefix    string
}

type ServiceConfig struct {
	ConfigSource ConfigSource
	Encryptor    Encryptor
	Store        Store
	Clock        Clock
	Location     *time.Location
	KeyPrefix    string
}

type RunResult struct {
	Changed      bool
	CurrentHash  string
	UploadedKey  string
	DeletedOld   bool
	PreviousKey  string
	SkippedEqual bool
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
	default:
	}

	return &Service{
		configSource: cfg.ConfigSource,
		encryptor:    cfg.Encryptor,
		store:        cfg.Store,
		clock:        cfg.Clock,
		location:     cfg.Location,
		keyPrefix:    strings.Trim(strings.TrimSpace(cfg.KeyPrefix), "/"),
	}, nil
}

func (s *Service) Run(ctx context.Context) (RunResult, error) {
	var result RunResult

	// 1) Calculate hash of current configuration.
	plaintext, err := s.configSource.ReadConfig(ctx)
	if err != nil {
		return result, fmt.Errorf("reading configuration: %w", err)
	}
	currentHash := hashBytes(plaintext)
	result.CurrentHash = currentHash

	// 2) Compare hash against previous uploaded backup metadata.
	previous, err := s.store.GetLatestBackup(ctx)
	if err != nil {
		return result, fmt.Errorf("getting latest backup metadata: %w", err)
	}
	if previous != nil {
		result.PreviousKey = previous.ObjectKey
	}
	if previous != nil && previous.Hash != "" && previous.Hash == currentHash {
		result.SkippedEqual = true
		return result, nil
	}

	// 3) Encrypt locally with age.
	encrypted, err := s.encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		return result, fmt.Errorf("encrypting configuration: %w", err)
	}

	// 4) Upload new encrypted backup.
	key := s.buildBackupObjectKey(s.clock.Now().In(s.location))
	if err := s.store.UploadBackup(ctx, key, encrypted, currentHash); err != nil {
		return result, fmt.Errorf("uploading encrypted backup: %w", err)
	}

	// 5) Delete old backup, only after successful upload.
	result.Changed = true
	result.UploadedKey = key
	deletedCount, err := s.store.CleanupBackups(ctx, key)
	if err != nil {
		return result, fmt.Errorf("cleaning up old backups while keeping %q: %w", key, err)
	}
	if deletedCount > 0 {
		result.DeletedOld = true
	}

	// 6) End.
	return result, nil
}

func (s *Service) buildBackupObjectKey(now time.Time) string {
	filename := "config-backup-" + now.Format("2006-01-02T15-04-05.000Z07-00") + ".json.age"
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
