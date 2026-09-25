package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore stores objects beneath a configured Server-owned directory. IDs
// are deliberately restricted to a conservative opaque form, preventing path
// traversal even if an internal caller is buggy.
type LocalStore struct{ root string }

func NewLocal(root string) (*LocalStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("storage root is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalStore{root: root}, nil
}

func (s *LocalStore) path(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid storage identifier")
	}
	return filepath.Join(s.root, id), nil
}

func (s *LocalStore) Put(ctx context.Context, id string, src io.Reader, size int64) error {
	if size < 0 {
		return fmt.Errorf("storage size must not be negative")
	}
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("storage object already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return fmt.Errorf("create temporary object: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()

	written, err := copyContext(ctx, tmp, io.LimitReader(src, size+1))
	if err != nil {
		return err
	}
	if written != size {
		return fmt.Errorf("storage object size mismatch")
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync storage object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close storage object: %w", err)
	}
	// Link is an atomic no-replace publish on the same filesystem. Rename
	// would silently replace an object another request just created after the
	// Stat check above; preserving an existing opaque ID is part of Store's
	// contract. The temporary file is removed by the deferred cleanup.
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("storage object already exists")
		}
		return fmt.Errorf("publish storage object: %w", err)
	}
	return nil
}

func (s *LocalStore) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (s *LocalStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err = os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

func (s *LocalStore) Exists(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := s.path(id)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := dst.Write(buf[:n])
			total += int64(wn)
			if writeErr != nil {
				return total, writeErr
			}
			if wn != n {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
