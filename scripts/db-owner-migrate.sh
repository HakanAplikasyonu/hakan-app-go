#!/usr/bin/env bash
# Üretim veritabanının sahibini süper kullanıcıdan (app) hakan_owner'a geçirir
# (issue #317, docs/runbook/db-sahip-gecisi.md). Tek işlem, tekrar
# çalıştırmak güvenli:
#   1. scripts/db-roles.sql (roller, parolalar),
#   2. veritabanındaki nesnelerin, politikaların (rls_system) ve varsayılan
#      yetkilerin sahibini taşır (scripts/db-owner.sql),
#   3. doğrular; bir şey eksikse hepsini geri alır.
#
#   ADMIN_URL=postgres://app:...@host:5432/app scripts/db-owner-migrate.sh [--dry-run]
#
# Ortam:
#   ADMIN_URL           hedef veritabanına süper kullanıcıyla (bugünkü DB_USER) bağlantı
#   FROM_ROLE           şimdiki sahip (varsayılan: ADMIN_URL'deki kullanıcı)
#   DB_OWNER_PASSWORD   hakan_owner'ın parolası (boşsa değiştirilmez)
#   DB_BACKUP_PASSWORD  hakan_backup'ın parolası (boşsa değiştirilmez)
# --dry-run: aynı işlemi yapar, doğrular, ne yaptığını listeler ve geri alır.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
die() { echo "db-owner-migrate: $*" >&2; exit 1; }
dry=off
case ${1:-} in
  --dry-run) dry=on ;;
  "") ;;
  -h|--help) sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p}' "$0"; exit 0 ;;
  *) die "unknown argument $1" ;;
esac
[[ -n ${ADMIN_URL:-} ]] || die "ADMIN_URL is required"
command -v psql >/dev/null || die "psql not found"
from=${FROM_ROLE:-$(psql "$ADMIN_URL" -XtAqc 'SELECT current_user')}
[[ $(psql "$ADMIN_URL" -XtAqc 'SELECT rolsuper FROM pg_roles WHERE rolname = current_user') == t ]] ||
  die "ADMIN_URL must connect as a superuser (the role that owns the database today)"
[[ $from != hakan_owner ]] || die "the database is already owned by hakan_owner"
args=(-X -v ON_ERROR_STOP=1 -v from="$from" -v to=hakan_owner -v dry="$dry" -v roles_file="$ROOT/scripts/db-roles.sql")
[[ -n ${DB_OWNER_PASSWORD:-} ]] && args+=(-v owner_password="$DB_OWNER_PASSWORD")
[[ -n ${DB_BACKUP_PASSWORD:-} ]] && args+=(-v backup_password="$DB_BACKUP_PASSWORD")
echo "db-owner-migrate: $from -> hakan_owner (dry-run: $dry)"
psql "$ADMIN_URL" "${args[@]}" -f "$ROOT/scripts/db-owner.sql"
