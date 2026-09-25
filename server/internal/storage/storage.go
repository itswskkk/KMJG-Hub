// Package storage provides the Server-owned persistent file abstraction.
// Application services use opaque identifiers only; clients never supply a
// filesystem path (docs/ARCHITECTURE.md "Storage Identifiers").
package storage

import (
	"context"
	"errors"
	"io"
)

var ErrNotFound = errors.New("storage: object not found")

// Store persists exactly size bytes from src under the supplied opaque ID.
// Implementations must reject an existing identifier rather than overwrite it.
type Store interface {
	Put(ctx context.Context, id string, src io.Reader, size int64) error
	Open(ctx context.Context, id string) (io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}
