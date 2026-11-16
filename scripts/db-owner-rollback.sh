#!/usr/bin/env bash
# Geri dönüş (issue #317): veritabanının sahibini hakan_owner'dan önceki
# süper kullanıcıya (app) geri verir. scripts/db-owner-migrate.sh'nin tersi;
# tek işlem, doğrulamalı, tekrar çalıştırmak güvenli. Roller (hakan_owner,
# hakan_backup) silinmez. Uygulamanın env değişkenlerini de geri alın
# (docs/runbook/db-sahip-gecisi.md, "Geri dönüş").
#
#   ADMIN_URL=postgres://app:...@host:5432/app scripts/db-owner-rollback.sh [--dry-run]
#
# Ortam: ADMIN_URL (hedef veritabanına süper kullanıcıyla), TO_ROLE (geri
# dönülecek sahip; varsayılan ADMIN_URL'deki kullanıcı).
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
die() { echo "db-owner-rollback: $*" >&2; exit 1; }
dry=off
case ${1:-} in
  --dry-run) dry=on ;;
  "") ;;
  -h|--help) sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p}' "$0"; exit 0 ;;
  *) die "unknown argument $1" ;;
esac
[[ -n ${ADMIN_URL:-} ]] || die "ADMIN_URL is required"
to=${TO_ROLE:-$(psql "$ADMIN_URL" -XtAqc 'SELECT current_user')}
[[ $to != hakan_owner ]] || die "TO_ROLE must be the previous owner, not hakan_owner"
[[ $(psql "$ADMIN_URL" -XtAqc 'SELECT rolsuper FROM pg_roles WHERE rolname = current_user') == t ]] ||
  die "ADMIN_URL must connect as a superuser"
echo "db-owner-rollback: hakan_owner -> $to (dry-run: $dry)"
psql "$ADMIN_URL" -X -v ON_ERROR_STOP=1 -v from=hakan_owner -v to="$to" -v dry="$dry" -f "$ROOT/scripts/db-owner.sql"
