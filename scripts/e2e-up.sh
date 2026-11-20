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
#   E2E_SKIP_FRONTEND=1  frontend'i hiç başlatma (yalnızca API; mobil Maestro işi, #300)
#   E2E_SA_EMAIL / E2E_SA_PASSWORD  oluşturulan platform yöneticisi
#   E2E_SKIP_SMOKE=1  ortam hazır olunca duman testini (scripts/smoke.mjs, #456) koşma
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

echo "== önceki koşu kapatılıyor"
# Eski API / frontend portu tutarsa yenisi bağlanamaz ve hazır olma
# denetimi eskisine cevap verir: önce önceki süreçleri ve compose'u kapat.
E2E_DIR="$DIR" "$ROOT/scripts/e2e-down.sh" >/dev/null 2>&1 || true
docker compose "${compose_args[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
for port in 8080 3000; do
  if curl -fsS -o /dev/null "http://127.0.0.1:$port/" 2>/dev/null || curl -fsS -o /dev/null "http://127.0.0.1:$port/healthz" 2>/dev/null; then
    echo "port $port hâlâ kullanımda: önceki süreci kapatın" >&2; exit 1
  fi
done

echo "== compose (temiz birimlerle)"
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
# Platform yöneticisi: yeni ofis açma senaryosu (ofis kaydı) için.
(cd "$ROOT/backend" && go run ./cmd/create-super-admin \
  -email "${E2E_SA_EMAIL:-sa@e2e.local}" -password "${E2E_SA_PASSWORD:-E2e-Admin-123!}" \
  -name Platform -surname Yönetici)

echo "== API"
(cd "$ROOT/backend" && go build -o "$DIR/server" ./cmd/server)
# Testler aynı demo hesaplarıyla, aynı IP'den çok kez giriş yapar; #180'in
# giriş sınırı (15 dk'da IP + e-posta başına 10) bir koşuda aşılıyordu.
# Sınırların kendisi backend birim testlerinde denetlenir (ratelimit). Env
# dosyasındaki (.env.example: 10 / 5) değerin üstüne yazılır; başka değer için
# E2E_LOGIN_LIMIT / E2E_REGISTER_LIMIT.
export RATE_LIMIT_LOGIN_LIMIT="${E2E_LOGIN_LIMIT:-1000}"
export RATE_LIMIT_REGISTER_LIMIT="${E2E_REGISTER_LIMIT:-1000}"
# Mükellef onayı (taxpayer-consents/approve) da IP başına 15 dk'da 20 ile
# sınırlı; addConsentedTaxpayer kullanan senaryolar çoğaldıkça 429 alınıyordu.
export RATE_LIMIT_TAXPAYER_ACCEPT_LIMIT="${E2E_TAXPAYER_ACCEPT_LIMIT:-1000}"
# Onay / davet bağlantısının önizlemesi de IP başına 15 dk'da 60; senaryo
# sayısı arttıkça koşunun sonunda 429 alınıyordu (kayıt formu açılmıyor).
export RATE_LIMIT_TAXPAYER_PREVIEW_LIMIT="${E2E_TAXPAYER_PREVIEW_LIMIT:-1000}"
# setsid: süreç grubu lideri olur, e2e-down grubu birlikte kapatır.
setsid nohup "$DIR/server" > "$DIR/server.log" 2>&1 < /dev/null &
echo $! > "$DIR/server.pid"
wait_for api "http://127.0.0.1:8080/readyz" 60

if [ "${E2E_SKIP_FRONTEND:-}" = "1" ]; then
  echo "== frontend atlandı (E2E_SKIP_FRONTEND=1)"
else
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
fi

if [ "${E2E_SKIP_SMOKE:-}" != "1" ]; then
  echo "== duman testi (scripts/smoke.mjs)"
  SMOKE_SA_EMAIL="${E2E_SA_EMAIL:-sa@e2e.local}" SMOKE_SA_PASSWORD="${E2E_SA_PASSWORD:-E2e-Admin-123!}" \
    node "$ROOT/scripts/smoke.mjs"
fi

echo "e2e ortamı hazır: API http://localhost:8080, panel http://localhost:3000 (Mailhog http://localhost:${MAILHOG_UI_PORT:-8025})"
