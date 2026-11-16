# Shared helpers for the backup scripts (sourced, not executed).
set -euo pipefail

BACKUP_DIR=${BACKUP_DIR:-/backups}

log() { printf '%s backup: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

# age recipients: BACKUP_AGE_RECIPIENTS, comma or space separated public keys
# (age1…). Only public keys live on the server; the identity (private key)
# that decrypts is kept offline and given to restore-* when needed.
#
# recipients_ok: status only (no exit), for the scheduler and health check.
recipients_ok() {
  local r n=0 list=${BACKUP_AGE_RECIPIENTS:-}
  for r in ${list//,/ }; do
    [[ $r == age1* ]] || return 1
    n=$((n + 1))
  done
  ((n > 0))
}

# require_recipients: exits when encryption is not configured. Backups are
# never written unencrypted.
require_recipients() {
  recipients_ok || die "BACKUP_AGE_RECIPIENTS must list age public keys (age1…): backups are never written unencrypted"
}

# age_recipient_args: one "-r" "key" pair per line, for mapfile.
age_recipient_args() {
  local r list=${BACKUP_AGE_RECIPIENTS:-}
  for r in ${list//,/ }; do printf -- '-r\n%s\n' "$r"; done
}

# rclone remote for the app object store (SeaweedFS S3), configured from env.
# Sets OBJECTS_REMOTE ("apps3:"). Call it directly, not in $(…): the rclone
# settings are exported into the current shell.
objects_remote() {
  : "${S3_ENDPOINT:?S3_ENDPOINT is required}" "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
  : "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"
  export RCLONE_CONFIG_APPS3_TYPE=s3 RCLONE_CONFIG_APPS3_PROVIDER=Other
  export RCLONE_CONFIG_APPS3_ENDPOINT=$S3_ENDPOINT RCLONE_CONFIG_APPS3_REGION=${S3_REGION:-us-east-1}
  export RCLONE_CONFIG_APPS3_ACCESS_KEY_ID=$S3_ACCESS_KEY RCLONE_CONFIG_APPS3_SECRET_ACCESS_KEY=$S3_SECRET_KEY
  export RCLONE_CONFIG_APPS3_FORCE_PATH_STYLE=true
  OBJECTS_REMOTE=apps3:
}

# Optional off-site copy target (any S3-compatible store). Sets OFFSITE_REMOTE,
# empty when BACKUP_OFFSITE_S3_BUCKET is not set. Call directly, like objects_remote.
offsite_remote() {
  OFFSITE_REMOTE=
  [[ -n ${BACKUP_OFFSITE_S3_BUCKET:-} ]] || return 0
  : "${BACKUP_OFFSITE_S3_ACCESS_KEY:?BACKUP_OFFSITE_S3_ACCESS_KEY is required}"
  : "${BACKUP_OFFSITE_S3_SECRET_KEY:?BACKUP_OFFSITE_S3_SECRET_KEY is required}"
  export RCLONE_CONFIG_OFFSITE_TYPE=s3 RCLONE_CONFIG_OFFSITE_PROVIDER=Other
  export RCLONE_CONFIG_OFFSITE_ENDPOINT=${BACKUP_OFFSITE_S3_ENDPOINT:-}
  export RCLONE_CONFIG_OFFSITE_REGION=${BACKUP_OFFSITE_S3_REGION:-us-east-1}
  export RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID=$BACKUP_OFFSITE_S3_ACCESS_KEY
  export RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY=$BACKUP_OFFSITE_S3_SECRET_KEY
  OFFSITE_REMOTE="offsite:${BACKUP_OFFSITE_S3_BUCKET}/${BACKUP_OFFSITE_PREFIX:-hakan}"
}

stamp() { date -u +%Y%m%dT%H%M%SZ; }
