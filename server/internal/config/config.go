// Package config loads KMJG Hub Server configuration from the deployment
// environment, per docs/ARCHITECTURE.md "Server Configuration".
package config

import (
	"encoding/base64"
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
	// FileStorageDir is the Server-owned root for persistent attachment data.
	FileStorageDir string
	// MaxUploadBytes and MaxProjectStorageBytes are administrator-controlled
	// attachment limits. Zero is never accepted: deployments must have bounds.
	MaxUploadBytes         int64
	MaxProjectStorageBytes int64

	// GitHub integration (docs/PRD.md "GitHub Authentication", "Project Git
	// Integration"). All optional: with no OAuth App client ID/secret the
	// Server runs with GitHub features reporting "not configured".
	GitHubClientID      string
	GitHubClientSecret  string
	GitHubWebhookSecret string
	// GitHubTokenEncryptionKey is the decoded 32-byte AES-256 key for
	// encrypting GitHub access tokens at rest
	// (KMJG_GITHUB_TOKEN_ENCRYPTION_KEY, standard base64). Nil when unset;
	// internal/app then generates a dev-only random key with a warning.
	GitHubTokenEncryptionKey []byte
}

// GitHubConfigured reports whether a GitHub OAuth App is configured.
func (c Config) GitHubConfigured() bool {
	return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

// Load builds a Config from environment variables, applying simple-for-v1
// defaults suitable for local development.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:     getEnv("KMJG_LISTEN_ADDR", ":8080"),
		DatabaseURL:    getEnv("KMJG_DATABASE_URL", ""),
		AllowedOrigins: splitCSV(getEnv("KMJG_ALLOWED_ORIGINS", "http://localhost:1420,tauri://localhost,http://tauri.localhost,https://tauri.localhost")),
		FileStorageDir: getEnv("KMJG_FILE_STORAGE_DIR", "/var/lib/kmjg-hub/files"),
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
	maxUpload, err := positiveInt64Env("KMJG_MAX_UPLOAD_BYTES", "26214400")
	if err != nil {
		return Config{}, err
	}
	maxProjectStorage, err := positiveInt64Env("KMJG_MAX_PROJECT_STORAGE_BYTES", "1073741824")
	if err != nil {
		return Config{}, err
	}
	cfg.MaxUploadBytes, cfg.MaxProjectStorageBytes = maxUpload, maxProjectStorage

	cfg.GitHubClientID = getEnv("KMJG_GITHUB_CLIENT_ID", "")
	cfg.GitHubClientSecret = getEnv("KMJG_GITHUB_CLIENT_SECRET", "")
	cfg.GitHubWebhookSecret = getEnv("KMJG_GITHUB_WEBHOOK_SECRET", "")
	if raw := getEnv("KMJG_GITHUB_TOKEN_ENCRYPTION_KEY", ""); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			return Config{}, fmt.Errorf("KMJG_GITHUB_TOKEN_ENCRYPTION_KEY must be 32 bytes, base64-encoded")
		}
		cfg.GitHubTokenEncryptionKey = key
	}

	return cfg, nil
}

func positiveInt64Env(key, fallback string) (int64, error) {
	v, err := strconv.ParseInt(getEnv(key, fallback), 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return v, nil
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
