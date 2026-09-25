#!/usr/bin/env bash
# Stops the local dev stack started by scripts/dev-up.sh.
#
# Usage:
#   scripts/dev-down.sh          stop server + client + PostgreSQL, keep data
#   scripts/dev-down.sh --wipe   also delete .dev-data (fresh state next run)
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname -- "$script_dir")
data_dir="$project_dir/.dev-data"
pg_data="$data_dir/postgres"

log() { printf '==> %s\n' "$1"; }

stop_pid_file() {
  name="$1"
  pid_file="$data_dir/$2"
  if [ -f "$pid_file" ]; then
    pid=$(cat "$pid_file")
    if kill -0 "$pid" 2>/dev/null; then
      log "Stopping $name (pid $pid)"
      # dev-up.sh starts these with setsid, so pid is also its process
      # group leader's id — kill the whole group (e.g. `npm run dev`'s
      # `vite` child), not just the launcher.
      kill -TERM -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 20); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.2
      done
      kill -9 -- "-$pid" 2>/dev/null || true
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
  else
    log "$name not running (no pid file)"
  fi
}

stop_pid_file "client" "client.pid"
stop_pid_file "server" "server.pid"

find_pg_bindir() {
  if command -v pg_ctl >/dev/null 2>&1; then
    dirname -- "$(command -v pg_ctl)"
    return 0
  fi
  for d in /usr/lib/postgresql/*/bin; do
    [ -x "$d/pg_ctl" ] && { echo "$d"; return 0; }
  done
  return 1
}

if [ -s "$pg_data/PG_VERSION" ]; then
  if pg_bindir=$(find_pg_bindir); then
    if "$pg_bindir/pg_ctl" -D "$pg_data" status >/dev/null 2>&1; then
      log "Stopping PostgreSQL"
      "$pg_bindir/pg_ctl" -D "$pg_data" stop -m fast >/dev/null
    else
      log "PostgreSQL not running"
    fi
  fi
fi

if [ "${1:-}" = "--wipe" ]; then
  log "Removing $data_dir"
  rm -rf "$data_dir"
fi
