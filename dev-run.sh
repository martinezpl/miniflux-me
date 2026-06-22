#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

if ! docker ps --format '{{.Names}}' | grep -q '^miniflux-db$'; then
  echo "Starting PostgreSQL..."
  # --restart unless-stopped + named volume => survives reboots and docker restarts.
  # Reuse the container if it exists (stopped), otherwise create it.
  if docker ps -a --format '{{.Names}}' | grep -q '^miniflux-db$'; then
    docker start miniflux-db
  else
    docker run --name miniflux-db -p 5432:5432 \
      -e POSTGRES_DB=miniflux2 -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
      -v miniflux-db-data:/var/lib/postgresql/data \
      --restart unless-stopped \
      -d postgres
  fi
  until docker exec miniflux-db pg_isready -U postgres >/dev/null 2>&1; do sleep 0.5; done
fi

LOG_DATE_TIME=1 LOG_LEVEL=debug \
RUN_MIGRATIONS=1 CREATE_ADMIN=1 \
ADMIN_USERNAME=admin ADMIN_PASSWORD=test123 \
PORT=9999 \
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost/miniflux2?sslmode=disable}" \
go run main.go
