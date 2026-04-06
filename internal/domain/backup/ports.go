package backup

import (
	"context"
	"time"
)

const HashMetadataKey = "config-sha512"

type ConfigSource interface {
	ReadConfig(ctx context.Context) ([]byte, error)
}

type Encryptor interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
}

type Store interface {
	GetLatestBackup(ctx context.Context) (*StoredBackup, error)
	UploadBackup(ctx context.Context, objectKey string, encrypted []byte, configHash string) error
	CleanupBackups(ctx context.Context, keepObjectKey string) (int, error)
}

type Clock interface {
	Now() time.Time
}

type StoredBackup struct {
	ObjectKey string
	Hash      string
}
