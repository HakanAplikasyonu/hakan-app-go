#!/usr/bin/env bash
# Uçtan uca test ortamı (#170): compose altyapısı, sıfırdan migration, demo
# seed (-reset), Go API (worker aynı süreçte) ve Next.js üretim derlemesi.
#
#   scripts/e2e-up.sh     # kur ve hazır olana kadar bekle
#   scripts/e2e-down.sh   # süreçleri durdur, compose'u birimleriyle sil
#
# Ayarlar (ortam değişkeni):
#   E2E_DIR           çalışma klasörü (env dosyası, pid'ler, loglar). Varsayılan e2e/.run
#   E2E_PG_PORT       compose Postgres'inin host portu (varsayılan 5432)
#   E2E_COMPOSE_FILES ek compose dosyaları, ":" ile ayrılmış (ör. yerel override)
#   E2E_SKIP_BUILD=1  frontend derlemesini atla (önceki .next kullanılır)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="${E2E_DIR:-$ROOT/e2e/.run}"
PG_PORT="${E2E_PG_PORT:-5432}"
mkdir -p "$DIR"
ENV_FILE="$DIR/e2e.env"

# .env.example + testte sabit kalması gerekenler. Son satır kazanır.
{
  cat "$ROOT/.env.example"
  echo
  echo "# --- e2e (scripts/e2e-up.sh) ---"
  echo "APP_ENV=development"
  echo "POSTGRES_PORT=$PG_PORT"
  echo "DB_HOST=127.0.0.1"
  echo "DB_PORT=$PG_PORT"
  echo "SEARCH_ENABLED=false"
  echo "QUEUE_ENABLED=true"
  echo "QUEUE_WORKER_INPROCESS=true"
  echo "NEXT_PUBLIC_REALTIME_ENABLED=true"
} > "$ENV_FILE"

# Makefile'daki load_env ile aynı: değerler olduğu gibi alınır (boşluklu
# değerler bölünmez).
load_env() {
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in \#*|'') continue ;; esac
    if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      export "${BASH_REMATCH[1]}=${BASH_REMATCH[2]}"
    fi
  done < "$1"
}
load_env "$ENV_FILE"

compose_args=(--env-file "$ENV_FILE" -f "$ROOT/compose.local.yml")
if [ -n "${E2E_COMPOSE_FILES:-}" ]; then
  IFS=':' read -ra extra <<< "$E2E_COMPOSE_FILES"
  for f in "${extra[@]}"; do compose_args+=(-f "$f"); done
fi
echo "$(printf '%q ' "${compose_args[@]}")" > "$DIR/compose-args"

wait_for() { # name url tries
  local name="$1" url="$2" tries="${3:-90}"
  for _ in $(seq 1 "$tries"); do
    if curl -fsS -o /dev/null "$url"; then echo "ready: $name"; return 0; fi
    sleep 2
  done
  echo "timeout: $name ($url)" >&2
  return 1
}

echo "== compose (temiz birimlerle)"
docker compose "${compose_args[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
docker compose "${compose_args[@]}" up -d postgres redis seaweedfs mailhog centrifugo
for svc in postgres redis seaweedfs centrifugo; do
  for _ in $(seq 1 60); do
    state="$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose "${compose_args[@]}" ps -q "$svc")" 2>/dev/null || true)"
    [ "$state" = "healthy" ] && break
    sleep 2
  done
  [ "$state" = "healthy" ] || { echo "unhealthy: $svc" >&2; docker compose "${compose_args[@]}" logs "$svc" | tail -40; exit 1; }
done
docker compose "${compose_args[@]}" up seaweedfs-init --exit-code-from seaweedfs-init

echo "== migration (0 → son) ve demo seed"
# migrate CLI kurulu değilse imajdaki sürümü (backend/Dockerfile) go run ile çalıştır.
MIGRATE_CMD="migrate"
if ! command -v migrate >/dev/null 2>&1; then
  mv="$(sed -n 's#^FROM migrate/migrate:\(v[0-9.]*\).*#\1#p' "$ROOT/backend/Dockerfile" | head -1)"
  MIGRATE_CMD="go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@${mv:-v4.19.1}"
fi
make -C "$ROOT" migrate-up ENV_FILE="$ENV_FILE" MIGRATE="$MIGRATE_CMD"
(cd "$ROOT/backend" && go run ./cmd/demo-seed -reset)

echo "== API"
(cd "$ROOT/backend" && go build -o "$DIR/server" ./cmd/server)
# setsid: süreç grubu lideri olur, e2e-down grubu birlikte kapatır.
setsid nohup "$DIR/server" > "$DIR/server.log" 2>&1 < /dev/null &
echo $! > "$DIR/server.pid"
wait_for api "http://127.0.0.1:8080/readyz" 60

echo "== frontend (next build && next start)"
if [ "${E2E_SKIP_BUILD:-}" != "1" ]; then
  (cd "$ROOT/frontend" && pnpm build > "$DIR/frontend-build.log" 2>&1) || { tail -60 "$DIR/frontend-build.log" >&2; exit 1; }
fi
(
  cd "$ROOT/frontend"
  setsid nohup ./node_modules/.bin/next start > "$DIR/frontend.log" 2>&1 < /dev/null &
  echo $! > "$DIR/frontend.pid"
)
wait_for frontend "http://127.0.0.1:3000/login" 90

echo "e2e ortamı hazır: http://localhost:3000 (Mailhog http://localhost:${MAILHOG_UI_PORT:-8025})"
