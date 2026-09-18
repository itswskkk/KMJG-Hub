// Package app wires the KMJG Hub Server's application services, transport,
// and persistence layers together, per docs/ARCHITECTURE.md
// "Server Internal Architecture":
//
//	HTTP -> Handlers -> Application Services -> Repositories -> PostgreSQL
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/config"
	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
	"github.com/itswskkk/KMJG-Hub/server/internal/store/postgres"
)

// sessionSweepInterval is how often the Hub re-validates every open
// WebSocket connection's session against authoritative Server state, so a
// session that becomes invalid by means other than an explicit logout
// (ordinary expiry) still eventually disconnects its real-time connections,
// per docs/ARCHITECTURE.md "Logout and Revocation". Explicit logout is
// handled immediately elsewhere (see httpapi.handleLogout); this sweep is
// the bounded-delay fallback for everything else.
const sessionSweepInterval = 60 * time.Second

// App holds the running application's dependencies.
type App struct {
	Pool     *pgxpool.Pool
	Router   http.Handler
	Realtime *realtime.Hub
}

// New connects to PostgreSQL, applies pending migrations, and wires the
// application services and HTTP router.
func New(ctx context.Context, cfg config.Config) (*App, error) {
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

	hub := realtime.NewHub(ctx)
	presenceService := &presence.Service{Membership: projectService, Hub: hub}
	hub.OnUserOnline = presenceService.HandleUserOnline
	hub.OnUserOffline = presenceService.HandleUserOffline

	go runSessionSweep(ctx, hub, authService)

	handlers := &httpapi.Handlers{
		Auth:     authService,
		Projects: projectService,
		Realtime: hub,
		Presence: presenceService,
	}
	router := httpapi.NewRouter(handlers, cfg.AllowedOrigins)

	return &App{Pool: pool, Router: router, Realtime: hub}, nil
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
func (a *App) Close() {
	a.Realtime.Shutdown()
	a.Pool.Close()
}
