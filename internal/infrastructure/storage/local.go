package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type Local struct {
	basePath string
}

func NewLocal(basePath string) Local {
	return Local{basePath: basePath}
}

func (l Local) Save(_ context.Context, name string, content []byte) (string, error) {
	if err := os.MkdirAll(l.basePath, 0o755); err != nil {
		return "", fmt.Errorf("create storage path: %w", err)
	}

	target := filepath.Join(l.basePath, filepath.Clean(name))
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return target, nil
}
