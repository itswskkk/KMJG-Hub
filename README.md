# KMJG Hub

KMJG Hub is a self-hosted collaboration hub for software teams, delivered as
a Go/PostgreSQL Server and a cross-platform Tauri desktop Client.

## Local development (no Docker)

Requirements: Go, Node.js, PostgreSQL (client + server binaries — `initdb`,
`pg_ctl`, `psql` — on `PATH` or under the standard
`/usr/lib/postgresql/<version>/bin` layout).

```sh
scripts/dev-up.sh    # starts a private PostgreSQL, the Go server, and the
                      # Vite dev client under ./.dev-data (gitignored)
scripts/dev-down.sh  # stops everything; add --wipe to also delete the data
```

`dev-up.sh` is idempotent: re-running it reuses existing database data and
skips services that are already running. It prints the Client, Server and
PostgreSQL addresses when done, and log locations for each. This is for
local development only — use the Docker Compose path below for anything
resembling a real deployment.

## Start the Server

Requirements: Docker Engine with Docker Compose and OpenSSL.

```sh
chmod +x scripts/setup-env.sh
./scripts/setup-env.sh
docker compose up --build -d
docker compose ps
```

The bootstrap script creates a mode-600 `.env` with a random PostgreSQL
password. `.env` is ignored by Git. Back up both named Docker volumes before
upgrades. The API is published on `http://localhost:8080` by default.

For a network or Internet deployment, terminate TLS at a trusted reverse
proxy and set `KMJG_ALLOWED_ORIGINS` and `KMJG_PUBLISHED_PORT` appropriately.
Session tokens must not travel over unencrypted networks.

## Build and verify the Client

```sh
cd client
npm ci
npm test
npm run build
npm run tauri build
```

The desktop Client stores saved-server metadata and repository mappings in
SQLite. Session tokens are stored separately in the operating system's
credential vault. Selecting a Project repository enables local Current Branch
detection; no Git-provider credential is required.

## Server verification

```sh
cd server
go test ./...
go vet ./...
```

Configuration and product details are documented in `.env.example` and the
authoritative files under `docs/`.
