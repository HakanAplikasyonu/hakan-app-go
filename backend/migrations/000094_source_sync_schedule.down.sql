DROP INDEX IF EXISTS idx_source_connections_due;
ALTER TABLE source_connections
    DROP CONSTRAINT IF EXISTS chk_source_connections_sync_failures,
    DROP CONSTRAINT IF EXISTS chk_source_connections_sync_interval,
    DROP COLUMN IF EXISTS sync_failures,
    DROP COLUMN IF EXISTS next_sync_at,
    DROP COLUMN IF EXISTS sync_interval_minutes;
