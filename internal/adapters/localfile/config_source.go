package localfile

import (
	"context"
	"fmt"
	"os"
)

type ConfigSource struct {
	path string
}

func NewConfigSource(path string) *ConfigSource {
	return &ConfigSource{path: path}
}

func (s *ConfigSource) ReadConfig(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	content, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	return content, nil
}
