package app

import (
	"context"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
	"github.com/itswskkk/KMJG-Hub/server/internal/session"
)

// fakeSessionRepository is a minimal, DB-free session.Repository stand-in.
// runSessionSweep only ever calls GetActiveByTokenHash (via
// auth.Service.SessionActive); the other two methods are unused here and
// exist only to satisfy the interface.
type fakeSessionRepository struct{}

func (fakeSessionRepository) Create(ctx context.Context, s *session.Session) error { return nil }

func (fakeSessionRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*session.Session, error) {
	return nil, session.ErrNotFound
}

func (fakeSessionRepository) Revoke(ctx context.Context, tokenHash string) error { return nil }

// TestRunSessionSweepExitsOnContextCancellation exercises the half of
// App.Close's lifecycle guarantee that belongs to the goroutine App starts
// directly (as opposed to the Hub's own internal goroutine, covered by
// realtime.TestWaitBlocksUntilTransitionDispatcherExits): cancelling the ctx
// runSessionSweep was given must make it return, which is what lets
// App.Close's a.wg.Wait() ever complete before Pool.Close() runs.
func TestRunSessionSweepExitsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hub := realtime.NewHub(ctx)
	authService := &auth.Service{Sessions: fakeSessionRepository{}}

	done := make(chan struct{})
	go func() {
		runSessionSweep(ctx, hub, authService)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("runSessionSweep returned before its context was cancelled")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSessionSweep did not exit after its context was cancelled")
	}
}
