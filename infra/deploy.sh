#!/usr/bin/env sh
set -eu

APP_DIR="${APP_DIR:-/opt/pov}"
REPOSITORY="${REPOSITORY:-https://github.com/jiminxmonster/pov.git}"

if [ ! -d "$APP_DIR/.git" ]; then
  git clone "$REPOSITORY" "$APP_DIR"
fi

cd "$APP_DIR"
git fetch origin main
git checkout main
git pull --ff-only origin main

if [ ! -f .env ]; then
  cp .env.vultr.example .env
  echo "Created $APP_DIR/.env. Set secure production values and run this script again." >&2
  exit 1
fi

if grep -Eq '^(POSTGRES_PASSWORD|SESSION_SECRET)=change-this' .env \
  || grep -Eq '^PUBLIC_ORIGIN=http://localhost$' .env; then
  echo "Refusing deployment: replace database/session placeholders and PUBLIC_ORIGIN in $APP_DIR/.env." >&2
  exit 1
fi

docker compose pull db caddy
docker compose build --pull frontend api
docker compose up -d --remove-orphans
docker compose ps
