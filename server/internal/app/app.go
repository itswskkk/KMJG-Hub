// Package app wires the KMJG Hub Server's application services, transport,
// and persistence layers together, per docs/ARCHITECTURE.md
// "Server Internal Architecture":
//
//	HTTP -> Handlers -> Application Services -> Repositories -> PostgreSQL
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/config"
	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
	"github.com/itswskkk/KMJG-Hub/server/internal/storage"
	"github.com/itswskkk/KMJG-Hub/server/internal/store/postgres"
	"github.com/itswskkk/KMJG-Hub/server/internal/task"
	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
)

// sessionSweepInterval is how often the Hub re-validates every open
// WebSocket connection's session against authoritative Server state, so a
// session that becomes invalid by means other than an explicit logout
// (ordinary expiry) still eventually disconnects its real-time connections,
// per docs/ARCHITECTURE.md "Logout and Revocation". Explicit logout is
// handled immediately elsewhere (see httpapi.handleLogout); this sweep is
// the bounded-delay fallback for everything else.
const sessionSweepInterval = 60 * time.Second

// ASSUMPTION A-260925-6: an hourly sweep (plus one at startup) is the
// product-acceptable precision for the 30-day deletion lifecycle.
const chatRetentionSweepInterval = time.Hour

// App holds the running application's dependencies.
type App struct {
	Pool     *pgxpool.Pool
	Router   http.Handler
	Realtime *realtime.Hub
	Storage  storage.Store

	// cancel stops every background goroutine App started (or that started
	// goroutines on App's behalf, such as the Hub's internal transition
	// dispatcher), independent of whatever context Close is eventually
	// called from. App owns this rather than relying on the ctx passed to
	// New: that ctx is the Server's process-lifetime signal context, which
	// on a non-signal shutdown path (e.g. the HTTP server failing to start)
	// is never cancelled, so cancelling it can't be relied on to stop these
	// goroutines before Close releases the resources they use.
	cancel context.CancelFunc
	// wg tracks goroutines App itself starts directly (currently just
	// runSessionSweep). Close waits on it before releasing Pool.
	wg sync.WaitGroup
	// closeOnce makes Close idempotent: callers (main.go's defer, plus any
	// caller of App.Close on an error path) may call it more than once.
	closeOnce sync.Once
}

// New connects to PostgreSQL, applies pending migrations, and wires the
// application services and HTTP router.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	fileStore, err := storage.NewLocal(cfg.FileStorageDir)
	if err != nil {
		return nil, fmt.Errorf("open file storage: %w", err)
	}
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := postgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	authService := &auth.Service{
		Users:      postgres.NewUserRepository(pool),
		Sessions:   postgres.NewSessionRepository(pool),
		SessionTTL: cfg.SessionTTL,
	}

	projectService := &project.Service{
		Repo: postgres.NewProjectRepository(pool),
	}
	invitationService := &invitation.Service{Repo: postgres.NewInvitationRepository(pool)}

	// appCtx is App's own child of ctx: it lets Close stop App's background
	// work unconditionally, rather than depending on ctx ever being
	// cancelled itself (see App.cancel).
	appCtx, cancel := context.WithCancel(ctx)

	hub := realtime.NewHub(appCtx)
	presenceService := &presence.Service{Membership: projectService, Hub: hub}
	chatService := &chat.Service{
		Repo:       postgres.NewChatRepository(pool),
		Membership: projectService,
		Publisher:  &chat.RealtimePublisher{Hub: hub},
		Storage:    fileStore, MaxUploadBytes: cfg.MaxUploadBytes,
		MaxProjectStorageBytes: cfg.MaxProjectStorageBytes,
	}
	taskService := &task.Service{
		Repo:       postgres.NewTaskRepository(pool),
		Membership: projectService,
		Publisher:  &task.RealtimePublisher{Hub: hub},
	}
	workContextService := &workcontext.Service{
		Repo: postgres.NewWorkContextRepository(pool), Online: hub,
		Membership: projectService, Publisher: &workcontext.RealtimePublisher{Hub: hub},
	}
	profileRepo := postgres.NewProfileRepository(pool, hub)
	profileService := &profile.Service{Repo: profileRepo, Membership: profileRepo}
	friendService := &friend.Service{
		Repo:      postgres.NewFriendRepository(pool),
		Publisher: &friend.RealtimePublisher{Hub: hub},
	}
	hub.OnUserOnline = presenceService.HandleUserOnline
	hub.OnUserOffline = presenceService.HandleUserOffline

	handlers := &httpapi.Handlers{
		Auth:         authService,
		Projects:     projectService,
		Invitations:  invitationService,
		Chat:         chatService,
		Tasks:        taskService,
		Profiles:     profileService,
		Friends:      friendService,
		Realtime:     hub,
		Presence:     presenceService,
		WorkContexts: workContextService,
	}
	router := httpapi.NewRouter(handlers, cfg.AllowedOrigins)

	a := &App{Pool: pool, Router: router, Realtime: hub, Storage: fileStore, cancel: cancel}
	a.wg.Add(2)
	go func() {
		defer a.wg.Done()
		runSessionSweep(appCtx, hub, authService)
	}()
	go func() {
		defer a.wg.Done()
		runChatRetentionSweep(appCtx, chatService)
	}()

	return a, nil
}

func runChatRetentionSweep(ctx context.Context, chatService *chat.Service) {
	cleanup := func() {
		if err := chatService.PurgeExpiredDeleted(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			slog.Error("app: purge expired deleted Project Chat messages", "error", err)
		}
	}
	cleanup()

	ticker := time.NewTicker(chatRetentionSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// runSessionSweep periodically closes any open WebSocket connection whose
// session is no longer active, until ctx is cancelled (Server shutdown).
func runSessionSweep(ctx context.Context, hub *realtime.Hub, authService *auth.Service) {
	ticker := time.NewTicker(sessionSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hub.Sweep(func(tokenHash string) bool {
				return authService.SessionActive(ctx, tokenHash)
			})
		}
	}
}

// Close releases the application's resources, including closing every open
// WebSocket connection so no goroutine or Client is left dangling after
// shutdown (docs/ARCHITECTURE.md's expectation of clean Server shutdown).
//
// It stops and waits for every App-owned background goroutine — the session
// sweep and the Hub's internal transition dispatcher — before releasing
// Pool, so neither can be caught mid-query against a Pool that has already
// been closed. Close is idempotent and safe to call more than once (e.g.
// cmd/server/main.go's defer ordering relative to its signal context's stop
// func is not guaranteed, so Close cannot assume ctx is already cancelled
// when it runs).
func (a *App) Close() {
	a.closeOnce.Do(func() {
		a.cancel()
		a.Realtime.Shutdown()
		a.wg.Wait()
		a.Realtime.Wait()
		a.Pool.Close()
	})
}
