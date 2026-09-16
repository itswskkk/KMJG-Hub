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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/config"
	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/store/postgres"
)

// App holds the running application's dependencies.
type App struct {
	Pool   *pgxpool.Pool
	Router http.Handler
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

	handlers := &httpapi.Handlers{Auth: authService, Projects: projectService}
	router := httpapi.NewRouter(handlers, cfg.AllowedOrigins)

	return &App{Pool: pool, Router: router}, nil
}

// Close releases the application's resources.
func (a *App) Close() {
	a.Pool.Close()
}
