#!/usr/bin/env bash
# Local development stack, no Docker required.
#
# Starts a private PostgreSQL instance, builds and runs the Go server, and
# starts the Vite dev server for the Client — all under this repo's
# .dev-data/ directory (gitignored), independent of any system PostgreSQL.
#
# Usage:
#   scripts/dev-up.sh          start everything (safe to re-run; reuses
#                               existing DB data and skips already-running
#                               services)
#   scripts/dev-down.sh        stop everything started by this script
#
# Override defaults with env vars: KMJG_DEV_DB_PORT, KMJG_DEV_SERVER_PORT,
# KMJG_DEV_CLIENT_PORT.
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname -- "$script_dir")
data_dir="$project_dir/.dev-data"
log_dir="$data_dir/logs"
pg_data="$data_dir/postgres"
pg_sock_dir="$data_dir/pg-sock"
tools_bin="$project_dir/.tools/bin"

db_port="${KMJG_DEV_DB_PORT:-5544}"
server_port="${KMJG_DEV_SERVER_PORT:-8090}"
client_port="${KMJG_DEV_CLIENT_PORT:-1420}"
db_url="postgres://kmjg@127.0.0.1:${db_port}/kmjg_hub?sslmode=disable"

mkdir -p "$log_dir" "$pg_sock_dir" "$tools_bin"

log() { printf '==> %s\n' "$1"; }

find_pg_bindir() {
  if command -v pg_ctl >/dev/null 2>&1; then
    dirname -- "$(command -v pg_ctl)"
    return 0
  fi
  # Debian/Ubuntu install layout: /usr/lib/postgresql/<version>/bin
  for d in /usr/lib/postgresql/*/bin; do
    if [ -x "$d/pg_ctl" ]; then
      echo "$d"
      return 0
    fi
  done
  return 1
}

pg_bindir=$(find_pg_bindir) || {
  echo "error: no PostgreSQL installation found (need initdb/pg_ctl/psql)." >&2
  echo "       install postgresql-16 (or any version) and re-run." >&2
  exit 1
}

# --- PostgreSQL ---------------------------------------------------------

if [ ! -s "$pg_data/PG_VERSION" ]; then
  log "Initializing local PostgreSQL data directory at $pg_data"
  "$pg_bindir/initdb" -D "$pg_data" -U kmjg --auth=trust -E UTF8 \
    >"$log_dir/initdb.log" 2>&1
fi

if "$pg_bindir/pg_ctl" -D "$pg_data" status >/dev/null 2>&1; then
  log "PostgreSQL already running"
else
  log "Starting PostgreSQL on 127.0.0.1:$db_port"
  "$pg_bindir/pg_ctl" -D "$pg_data" \
    -o "-p $db_port -k $pg_sock_dir -c listen_addresses=127.0.0.1" \
    -l "$log_dir/postgres.log" start
fi

for _ in $(seq 1 30); do
  "$pg_bindir/pg_isready" -h 127.0.0.1 -p "$db_port" -U kmjg >/dev/null 2>&1 && break
  sleep 0.5
done

if ! "$pg_bindir/psql" -h 127.0.0.1 -p "$db_port" -U kmjg -d postgres -tAc \
    "SELECT 1 FROM pg_database WHERE datname='kmjg_hub'" | grep -q 1; then
  log "Creating kmjg_hub database"
  "$pg_bindir/createdb" -h 127.0.0.1 -p "$db_port" -U kmjg kmjg_hub
fi

# --- Server (Go) ---------------------------------------------------------

server_pid_file="$data_dir/server.pid"
if [ -f "$server_pid_file" ] && kill -0 "$(cat "$server_pid_file")" 2>/dev/null; then
  log "Server already running (pid $(cat "$server_pid_file"))"
else
  log "Building server"
  (cd "$project_dir/server" && go build -o "$tools_bin/kmjg-server" ./cmd/server)

  log "Starting server on 127.0.0.1:$server_port"
  (
    cd "$project_dir/server"
    KMJG_LISTEN_ADDR="127.0.0.1:$server_port" \
    KMJG_DATABASE_URL="$db_url" \
    KMJG_ALLOWED_ORIGINS="http://localhost:$client_port,tauri://localhost,http://tauri.localhost,https://tauri.localhost" \
    KMJG_FILE_STORAGE_DIR="$data_dir/files" \
    setsid nohup "$tools_bin/kmjg-server" >"$log_dir/server.log" 2>&1 &
    echo $! > "$server_pid_file"
  )
fi

for _ in $(seq 1 30); do
  curl -fs "http://127.0.0.1:$server_port/api/v1/health" >/dev/null 2>&1 && break
  sleep 0.5
done

# --- Client (Vite dev server) --------------------------------------------

client_pid_file="$data_dir/client.pid"
if [ -f "$client_pid_file" ] && kill -0 "$(cat "$client_pid_file")" 2>/dev/null; then
  log "Client already running (pid $(cat "$client_pid_file"))"
else
  if [ ! -d "$project_dir/client/node_modules" ]; then
    log "Installing client dependencies (npm ci)"
    (cd "$project_dir/client" && npm ci)
  fi

  log "Starting client dev server on http://localhost:$client_port"
  (
    cd "$project_dir/client"
    # `npm run dev` forks a `vite` child; setsid puts the whole tree in its
    # own process group so dev-down.sh can kill it as a unit (killing just
    # npm's pid leaves vite running and the port held).
    setsid nohup npm run dev >"$log_dir/client.log" 2>&1 &
    echo $! > "$client_pid_file"
  )
fi

cat <<EOF

KMJG Hub dev stack is up:
  Client:   http://localhost:$client_port
  Server:   http://localhost:$server_port  (health: /api/v1/health)
  Postgres: 127.0.0.1:$db_port  (db: kmjg_hub, user: kmjg, no password)

Logs:  $log_dir/{postgres,server,client}.log
Stop:  scripts/dev-down.sh
EOF
