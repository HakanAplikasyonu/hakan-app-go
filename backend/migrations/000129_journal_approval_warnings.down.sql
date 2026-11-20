ALTER TABLE journal_drafts
    DROP COLUMN IF EXISTS approval_note,
    DROP COLUMN IF EXISTS approval_warnings;
