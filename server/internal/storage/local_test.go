package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalStoreRoundTripAndOpaqueIDs(t *testing.T) {
	s, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "attachment-1", strings.NewReader("hello"), 5); err != nil {
		t.Fatal(err)
	}
	r, err := s.Open(ctx, "attachment-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	if err := s.Put(ctx, "../outside", strings.NewReader("x"), 1); err == nil {
		t.Fatal("expected unsafe ID error")
	}
	if err := s.Delete(ctx, "attachment-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "attachment-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestLocalStoreRejectsSizeMismatchWithoutPublishing(t *testing.T) {
	s, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = s.Put(context.Background(), "attachment-1", strings.NewReader("short"), 6)
	if err == nil {
		t.Fatal("expected size mismatch")
	}
	exists, err := s.Exists(context.Background(), "attachment-1")
	if err != nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func TestLocalStoreNeverOverwritesAnObject(t *testing.T) {
	s, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "attachment-1", strings.NewReader("first"), 5); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "attachment-1", strings.NewReader("other"), 5); err == nil {
		t.Fatal("expected duplicate object error")
	}
	r, err := s.Open(ctx, "attachment-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	r.Close()
	if err != nil || string(got) != "first" {
		t.Fatalf("got %q, err=%v", got, err)
	}
}
