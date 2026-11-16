-- Yevmiye taslak satırı bir fatura kalemine bağlanabilir (issue #239):
-- source_kind = 'invoice_item', source_id = invoice_items.id. Onaylanan
-- hesap kodu önerisi bu satırlara yazılır.
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'invoice_item', 'bank_transaction', 'receipt', 'manual'));
