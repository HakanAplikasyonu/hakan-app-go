UPDATE journal_draft_lines SET source_kind = 'invoice' WHERE source_kind = 'invoice_item';
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'bank_transaction', 'receipt', 'manual'));
