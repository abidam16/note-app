package storage

import "context"

type FileStorage interface {
	Save(ctx context.Context, name string, content []byte) (string, error)
}
