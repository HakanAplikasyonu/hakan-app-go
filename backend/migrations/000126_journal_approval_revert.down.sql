ALTER TABLE journal_drafts
    DROP COLUMN approval_revert_reason,
    DROP COLUMN approval_reverted_at,
    DROP COLUMN approval_reverted_by;
