ALTER TABLE receipt_captures
    DROP CONSTRAINT IF EXISTS chk_receipt_captures_reject_reason_text,
    DROP CONSTRAINT IF EXISTS chk_receipt_captures_reject_reason_code,
    DROP COLUMN IF EXISTS reject_reason_text,
    DROP COLUMN IF EXISTS reject_reason_code;
