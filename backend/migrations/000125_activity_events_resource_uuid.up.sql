-- Per-record audit history (#565): the bank movement panel lists the events
-- of one resource; without this every lookup scans the organization's log.
CREATE INDEX IF NOT EXISTS activity_events_resource_uuid_idx
    ON activity_events (resource_uuid, created_at DESC)
    WHERE resource_uuid IS NOT NULL;
