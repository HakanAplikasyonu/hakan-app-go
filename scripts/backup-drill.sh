#!/usr/bin/env bash
# Backup / restore drill (#178, docs/yedekleme.md). CI job backup-drill.
#
#   docker build -t local/hakan-backup:main deploy/backup
#   scripts/backup-drill.sh
#
# 1. Migrates a fresh Postgres, backs up the still-empty bucket, then adds
#    marker rows and objects.
# 2. Runs backup-run (encrypted dump + roles + manifest, encrypted bucket tar).
# 3. Checks the backup files hold no plaintext marker.
# 4. Restores the dump into a second, empty Postgres cluster and verifies the
#    schema version and the row count of every table (verify-db).
# 5. Restores the objects into a new bucket and compares their content.
# Everything is removed afterwards (DRILL_KEEP=1 keeps it).
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
export DRILL_DIR
DRILL_DIR=$(mktemp -d)
chmod 0777 "$DRILL_DIR"
MARKER="drill-marker-$(date +%s)-$RANDOM"
compose() { docker compose -f deploy/backup/drill.compose.yml "$@"; }
die() { echo "backup-drill: FAILED: $*" >&2; exit 1; }
cleanup() {
  local rc=$?
  if ((rc != 0)); then compose logs --no-color --tail 60 >&2 || true; fi
  if [[ ${DRILL_KEEP:-0} != 1 ]]; then
    compose down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$DRILL_DIR" 2>/dev/null || true
  fi
  exit "$rc"
}
trap cleanup EXIT

image=${DRILL_BACKUP_IMAGE:-local/hakan-backup:main}
docker image inspect "$image" >/dev/null 2>&1 || die "image $image not built (docker build -t $image deploy/backup)"

# The identity (private key) stays in the drill dir, like an offline key.
docker run --rm --entrypoint age-keygen -v "$DRILL_DIR:/drill" "$image" -o /drill/identity.txt 2>/dev/null
export DRILL_AGE_RECIPIENT
DRILL_AGE_RECIPIENT=$(docker run --rm --entrypoint age-keygen -v "$DRILL_DIR:/drill" "$image" -y /drill/identity.txt)

compose up -d --wait postgres postgres-restore seaweedfs
compose run --rm migrate
compose up -d backup
b() { compose exec -T backup "$@"; }

# A fresh install has an empty bucket: the object backup must still work.
b bash -c 'source /usr/local/bin/backup-lib.sh; objects_remote; rclone mkdir "${OBJECTS_REMOTE}app"'
b env BACKUP_DIR=/drill/empty backup-objects
b sh -c 'ls /drill/empty/objects/*.tar.age' >/dev/null || die "empty bucket: no archive"
echo "backup-drill: empty bucket backed up"

# Marker data: one table row and three objects.
b psql -Xq -v ON_ERROR_STOP=1 -c "INSERT INTO app_releases (version, git_sha, notes) VALUES ('drill', 'drill', '$MARKER')"
b bash -c 'source /usr/local/bin/backup-lib.sh; objects_remote; r=$OBJECTS_REMOTE; rclone mkdir "${r}app"
  for i in 1 2 3; do printf "%s object %s\n" "$0" "$i" | rclone rcat "${r}app/drill/obj-$i.txt"; done' "$MARKER"

b env BACKUP_DIR=/drill/backups backup-run
dump=$(b sh -c 'ls /drill/backups/db/*.dump.age' | tr -d '\r')
archive=$(b sh -c 'ls /drill/backups/objects/*.tar.age' | tr -d '\r')
[[ -n $dump && -n $archive ]] || die "backup files missing"

if grep -rqa "$MARKER" "$DRILL_DIR/backups"; then
  die "plaintext marker found in the backup files"
fi
echo "backup-drill: backup files are encrypted"

b env PGHOST=postgres-restore restore-db --identity /drill/identity.txt "$dump"
got=$(b env PGHOST=postgres-restore psql -XtAq -c "SELECT notes FROM app_releases WHERE version = 'drill'" | tr -d '\r')
[[ $got == "$MARKER" ]] || die "marker row not restored (got '$got')"
# A second restore into a non-empty database is refused without --clean.
if b env PGHOST=postgres-restore restore-db --identity /drill/identity.txt "$dump" 2>/dev/null; then
  die "restore into a non-empty database was not refused"
fi
echo "backup-drill: database restored and verified"

b restore-objects --identity /drill/identity.txt --bucket restored "$archive"
b bash -c 'source /usr/local/bin/backup-lib.sh; objects_remote; r=$OBJECTS_REMOTE
  rclone check --one-way "${r}app" "${r}restored"'
echo "backup-drill: objects restored and verified"
echo "backup-drill: ok"
