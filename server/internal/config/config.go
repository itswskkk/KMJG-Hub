// Package config loads KMJG Hub Server configuration from the deployment
// environment, per docs/ARCHITECTURE.md "Server Configuration".
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds deployment-provided KMJG Hub Server configuration.
type Config struct {
	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string
	// AllowedOrigins lists origins permitted to make cross-origin requests
	// to the HTTP API (the Desktop Client's Vite dev server, Tauri webview).
	AllowedOrigins []string
	// SessionTTL is how long a newly created session remains valid.
	SessionTTL time.Duration
}

// Load builds a Config from environment variables, applying simple-for-v1
// defaults suitable for local development.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:     getEnv("KMJG_LISTEN_ADDR", ":8080"),
		DatabaseURL:    getEnv("KMJG_DATABASE_URL", ""),
		AllowedOrigins: splitCSV(getEnv("KMJG_ALLOWED_ORIGINS", "http://localhost:1420,tauri://localhost,http://tauri.localhost,https://tauri.localhost")),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("KMJG_DATABASE_URL is required")
	}

	ttlHours := getEnv("KMJG_SESSION_TTL_HOURS", "720") // 30 days
	hours, err := strconv.Atoi(ttlHours)
	if err != nil || hours <= 0 {
		return Config{}, fmt.Errorf("KMJG_SESSION_TTL_HOURS must be a positive integer, got %q", ttlHours)
	}
	cfg.SessionTTL = time.Duration(hours) * time.Hour

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
