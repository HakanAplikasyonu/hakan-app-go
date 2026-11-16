#!/usr/bin/env bash
# Production stack smoke test on an empty Docker host (#177, docs/deploy.md).
#
#   scripts/prod-smoke.sh
#
# Generates a throwaway .env.server (http://localhost, fresh secrets), starts
# deploy/compose.prod.yml with the images already in the local Docker cache
# (IMAGE_PULL_POLICY=missing), then checks: migrate exited 0 and recorded a
# schema version, backend /readyz, the proxy serves the frontend /health and
# reaches the API through the BFF. Everything (containers, volumes, the env
# file) is removed afterwards. Refuses to run when .env.server already exists.
#
# Env: SMOKE_IMAGE_REPO (default local/hakan: images local/hakan-backend:main,
# local/hakan-frontend:main), SMOKE_TIMEOUT seconds (default 420),
# SMOKE_KEEP=1 to leave the stack running for debugging.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
ENV_FILE=.env.server
REPO=${SMOKE_IMAGE_REPO:-local/hakan}
TIMEOUT=${SMOKE_TIMEOUT:-420}
PREFIX=hakansmoke
export IMAGE_PULL_POLICY=missing

die() { echo "prod-smoke: $*" >&2; exit 1; }
[[ ! -e $ENV_FILE ]] || die "$ENV_FILE exists; refusing to overwrite a real env file"

compose() { docker compose -f deploy/compose.prod.yml "$@"; }

cleanup() {
  local rc=$?
  if ((rc != 0)); then
    echo "prod-smoke: FAILED, service state and recent logs:" >&2
    compose ps -a >&2 || true
    compose logs --no-color --tail 80 >&2 || true
  fi
  if [[ ${SMOKE_KEEP:-0} != 1 ]]; then
    compose down -v --remove-orphans >/dev/null 2>&1 || true
    rm -f "$ENV_FILE" "$ENV_FILE.bak"
  fi
  exit "$rc"
}
trap cleanup EXIT

scripts/prod-env.sh init --file "$ENV_FILE" --yes \
  --set APP_URL=http://localhost \
  --set NAME_PREFIX=$PREFIX \
  --set IMAGE_REPO="$REPO" \
  --set CENTRIFUGO_WS_URL=ws://realtime.localhost/connection/websocket \
  --set PROXY_REALTIME_SITE=http://realtime.localhost

for img in "$REPO-backend:main" "$REPO-frontend:main"; do
  docker image inspect "$img" >/dev/null 2>&1 || die "image $img not in the local cache (build it first)"
done

compose config --quiet
compose up -d

# Wait for the long-running services to be healthy.
deadline=$((SECONDS + TIMEOUT))
for svc in backend frontend proxy; do
  while :; do
    cid=$(compose ps -q "$svc" || true)
    state=$([[ -n $cid ]] && docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" || echo missing)
    [[ $state == healthy || ($svc == proxy && $state == running) ]] && break
    # A one-shot dependency that failed never recovers: stop early.
    for job in migrate seaweedfs-init; do
      jid=$(compose ps -aq "$job" || true)
      if [[ -n $jid && $(docker inspect -f '{{.State.Status}} {{.State.ExitCode}}' "$jid") == exited\ [1-9]* ]]; then
        die "$job failed"
      fi
    done
    ((SECONDS < deadline)) || die "$svc not healthy after ${TIMEOUT}s (state: $state)"
    sleep 3
  done
  echo "prod-smoke: $svc $state"
done

mid=$(compose ps -aq migrate)
[[ $(docker inspect -f '{{.State.ExitCode}}' "$mid") == 0 ]] || die "migrate did not exit 0"
version=$(compose exec -T postgres sh -c 'psql -tA -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT version FROM schema_migrations WHERE NOT dirty"')
latest=$(find backend/migrations -name '*.up.sql' | sed -E 's#.*/0*([0-9]+)_.*#\1#' | sort -n | tail -1)
[[ $version == "$latest" ]] || die "schema version '$version', want $latest"
echo "prod-smoke: migrations at version $version"

compose exec -T backend wget -q -O /dev/null http://localhost:8080/readyz || die "backend /readyz failed"
echo "prod-smoke: backend ready"

curl -fsS -o /dev/null http://localhost/health || die "proxy -> frontend /health failed"
code=$(curl -sS -o /dev/null -w '%{http_code}' http://localhost/api/v1/app/config)
[[ $code == 200 ]] || die "proxy -> BFF -> API /v1/app/config returned $code"
echo "prod-smoke: proxy -> frontend -> API ok"
echo "prod-smoke: ok"
