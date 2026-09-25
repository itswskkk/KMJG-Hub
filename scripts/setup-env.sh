#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname -- "$script_dir")
env_file="$project_dir/.env"

if [ -f "$env_file" ]; then
  echo ".env already exists; leaving it unchanged."
  exit 0
fi

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required to generate a deployment password." >&2
  exit 1
}

password=$(openssl rand -hex 32)
umask 077
sed "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$password/" "$project_dir/.env.example" > "$env_file"
echo "Created .env with a random database password (mode 600)."
