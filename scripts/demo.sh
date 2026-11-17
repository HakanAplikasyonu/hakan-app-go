#!/usr/bin/env bash
# Pilot demosu tek komutla (#456): compose altyapısı, temiz veritabanı,
# migration (0 → son), demo seed ve örnek kullanıcılar (demo ofisi, mükellef
# kullanıcısı, platform yöneticisi). Senaryo: docs/kullanim/demo-senaryosu.md.
#
#   make demo                     # altyapıyı kaldır, veritabanını SIFIRLA, kur
#   make demo ARGS=--no-infra     # altyapı zaten çalışıyor (yalnızca DB + seed)
#
# DİKKAT: ENV_FILE'daki veritabanının public şeması silinip yeniden kurulur.
# APP_ENV=production'da ve yerel olmayan bir DB_HOST'ta çalışmaz (bilerek
# uzak bir test veritabanı için DEMO_ALLOW_REMOTE_DB=1).
#
# Ayarlar (ortam değişkeni):
#   ENV_FILE                 env dosyası (varsayılan .env, yoksa .env.example)
#   DEMO_PASSWORD            demo hesaplarının parolası (varsayılan Demo1234!)
#   DEMO_CLICKFIX_WIDGET_ID  demo ofisinin destek widget kimliği (boş: kapalı)
#   SA_EMAIL / SA_PASSWORD   platform yöneticisi (varsayılan admin@example.com / Password1)
#   DEMO_COMPOSE_FILES       ek compose dosyaları, ":" ile ayrılmış (ör. yerel override)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INFRA=1
for arg in "$@"; do
  case "$arg" in
    --no-infra) INFRA=0 ;;
    -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
    *) echo "bilinmeyen seçenek: $arg" >&2; exit 2 ;;
  esac
done

ENV_FILE="${ENV_FILE:-}"
if [ -z "$ENV_FILE" ]; then
  if [ -f "$ROOT/.env" ]; then ENV_FILE="$ROOT/.env"; else ENV_FILE="$ROOT/.env.example"; fi
fi
case "$ENV_FILE" in /*) ;; *) ENV_FILE="$ROOT/$ENV_FILE" ;; esac
[ -f "$ENV_FILE" ] || { echo "env dosyası yok: $ENV_FILE" >&2; exit 1; }
echo "== env: $ENV_FILE"

# Makefile'daki load_env ile aynı: değerler olduğu gibi alınır.
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in \#*|'') continue ;; esac
  if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
    export "${BASH_REMATCH[1]}=${BASH_REMATCH[2]}"
  fi
done < "$ENV_FILE"

if [ "${APP_ENV:-}" = "production" ]; then
  echo "APP_ENV=production: demo kurulmaz" >&2; exit 1
fi
DB_HOST="${DB_HOST:-127.0.0.1}" DB_PORT="${DB_PORT:-5432}" DB_USER="${DB_USER:-app}"
DB_PASSWORD="${DB_PASSWORD:-change_me}" DB_NAME="${DB_NAME:-app}" DB_SSLMODE="${DB_SSLMODE:-disable}"
case "$DB_HOST" in
  127.0.0.1|localhost|::1|postgres) ;;
  *) if [ "${DEMO_ALLOW_REMOTE_DB:-}" != "1" ]; then
       echo "DB_HOST=$DB_HOST yerel değil; veritabanı silineceği için durdu (DEMO_ALLOW_REMOTE_DB=1 ile zorlayın)" >&2; exit 1
     fi ;;
esac

compose=(docker compose --env-file "$ENV_FILE" -f "$ROOT/compose.local.yml")
if [ -n "${DEMO_COMPOSE_FILES:-}" ]; then
  IFS=':' read -ra extra <<< "$DEMO_COMPOSE_FILES"
  for f in "${extra[@]}"; do compose+=(-f "$f"); done
fi
if [ "$INFRA" = "1" ]; then
  echo "== altyapı (compose.local.yml)"
  "${compose[@]}" up -d
  for svc in postgres redis; do
    state=""
    for _ in $(seq 1 60); do
      state="$(docker inspect -f '{{.State.Health.Status}}' "$("${compose[@]}" ps -q "$svc")" 2>/dev/null || true)"
      [ "$state" = "healthy" ] && break
      sleep 2
    done
    [ "$state" = "healthy" ] || { echo "hazır değil: $svc" >&2; "${compose[@]}" logs "$svc" | tail -40; exit 1; }
  done
fi

echo "== temiz veritabanı ($DB_NAME @ $DB_HOST:$DB_PORT)"
reset_sql='DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;'
if [ "$INFRA" = "1" ]; then
  "${compose[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -q -U "$DB_USER" -d "$DB_NAME" -c "$reset_sql"
elif command -v psql >/dev/null 2>&1; then
  PGPASSWORD="$DB_PASSWORD" psql -v ON_ERROR_STOP=1 -q -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "$reset_sql"
else
  echo "--no-infra için psql gerekli" >&2; exit 1
fi

echo "== migration (0 → son)"
# migrate CLI kurulu değilse imajdaki sürümü (backend/Dockerfile) go run ile çalıştır.
MIGRATE_CMD="migrate"
if ! command -v migrate >/dev/null 2>&1; then
  mv="$(sed -n 's#^FROM migrate/migrate:\(v[0-9.]*\).*#\1#p' "$ROOT/backend/Dockerfile" | head -1)"
  MIGRATE_CMD="go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@${mv:-v4.19.1}"
fi
make -s -C "$ROOT" migrate-up ENV_FILE="$ENV_FILE" MIGRATE="$MIGRATE_CMD"

echo "== demo seed"
(cd "$ROOT/backend" && go run ./cmd/demo-seed -reset)

echo "== platform yöneticisi"
SA_EMAIL="${SA_EMAIL:-admin@example.com}"
SA_PASSWORD="${SA_PASSWORD:-Password1}"
(cd "$ROOT/backend" && go run ./cmd/create-super-admin -email "$SA_EMAIL" -password "$SA_PASSWORD" -name Platform -surname Yönetici)

cat <<MSG

Demo hazır. Uygulamayı başlatın:
  make backend-dev     # API :8080 (ayrı terminal)
  make frontend-dev    # panel :3000 (ayrı terminal)

Girişler:
  Ofis paneli   http://localhost:3000/login         sahip@demo.hakan.local, musavir@…, stajyer@…
  Mükellef      http://localhost:3000/login         mukellef@demo.hakan.local (/m)
  Platform      http://localhost:3000/platform/login  $SA_EMAIL / $SA_PASSWORD
  Demo parolası: ${DEMO_PASSWORD:-Demo1234!}
Senaryo: docs/kullanim/demo-senaryosu.md
MSG
