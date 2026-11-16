# Backup / restore image (#178, docs/yedekleme.md): pg_dump/pg_restore of the
# same major as compose.prod.yml's Postgres, age (encryption) and rclone
# (object store and off-site copies). Built and published by CI from
# deploy/images.json like the app images.
FROM postgres:18-alpine

RUN apk add --no-cache age rclone bash coreutils jq tzdata

COPY bin/ /usr/local/bin/
RUN chmod 0755 /usr/local/bin/backup-* /usr/local/bin/restore-* /usr/local/bin/verify-db

ENV TZ=Europe/Istanbul \
    BACKUP_DIR=/backups

VOLUME ["/backups"]
HEALTHCHECK --interval=5m --timeout=10s --start-period=1m CMD ["backup-health"]
ENTRYPOINT ["backup-entrypoint"]
# Clear the postgres image CMD: no arguments means "run the scheduler".
CMD []
