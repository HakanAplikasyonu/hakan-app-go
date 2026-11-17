-- M15 tablolarında yabancı anahtar indeksleri (issue #453). Üst kayıt
-- silinince (ON DELETE CASCADE / SET NULL) ya da üst kayda göre
-- arandığında bu yabancı anahtarların taranması tablo taraması
-- gerektiriyordu: anahtar sütunlarıyla başlayan bir indeks yoktu.
-- Kullanıcıya bağlı denetim sütunları (created_by, updated_by, ...) ve
-- RESTRICT ile silinmesi engellenen taxpayer_id tek başına bilerek
-- indekslenmez: kullanıcı ve mükellef kayıtları silinmez, bu sütunlara
-- göre sorgu yoktur.

-- Banka hareketi silinince fiş önerisi (CASCADE).
CREATE INDEX IF NOT EXISTS idx_bank_transaction_postings_transaction
    ON bank_transaction_postings (taxpayer_id, bank_transaction_id);

-- Banka hesabı (cash box) silinince mizan hesabı eşlemesi (CASCADE).
CREATE INDEX IF NOT EXISTS idx_bank_ledger_accounts_cash_box
    ON bank_ledger_accounts (taxpayer_id, cash_box_id);

-- Dışa aktarım silinince aktarım görevleri (CASCADE); mevcut benzersiz
-- indeks yalnızca canlı durumları kapsar.
CREATE INDEX IF NOT EXISTS idx_accounting_transfers_batch
    ON accounting_transfers (engagement_id, export_batch_id);

-- Aktarımı üstlenen masaüstü ajanı silinince (SET NULL).
CREATE INDEX IF NOT EXISTS idx_accounting_transfers_agent
    ON accounting_transfers (agent_id) WHERE agent_id IS NOT NULL;

-- Bordro dönemine ve ödemeye bağlı SGK primleri.
CREATE INDEX IF NOT EXISTS idx_sgk_premiums_run
    ON sgk_premiums (taxpayer_id, payroll_run_id) WHERE payroll_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sgk_premiums_bank_transaction
    ON sgk_premiums (bank_transaction_id) WHERE bank_transaction_id IS NOT NULL;

-- Masraf / Z onayından doğan fiş taslağı.
CREATE INDEX IF NOT EXISTS idx_receipt_postings_journal
    ON receipt_postings (engagement_id, journal_draft_id) WHERE journal_draft_id IS NOT NULL;

-- Bordro dönemi onayından doğan fiş taslağı (SET NULL).
CREATE INDEX IF NOT EXISTS idx_payroll_runs_journal
    ON payroll_runs (journal_draft_id) WHERE journal_draft_id IS NOT NULL;

-- Bordro belgeleri: personel ve döneme göre (SET NULL).
CREATE INDEX IF NOT EXISTS idx_payroll_documents_employee
    ON payroll_documents (taxpayer_id, employee_id) WHERE employee_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payroll_documents_run
    ON payroll_documents (taxpayer_id, payroll_run_id) WHERE payroll_run_id IS NOT NULL;

-- Mutabakat koşusunun mizanları (SET NULL).
CREATE INDEX IF NOT EXISTS idx_reconciliation_runs_import
    ON reconciliation_runs (organization_id, trial_balance_import_id) WHERE trial_balance_import_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_reconciliation_runs_previous
    ON reconciliation_runs (organization_id, previous_import_id) WHERE previous_import_id IS NOT NULL;

-- WhatsApp yüklemesinden doğan ek, fiş ve belge görevi.
CREATE INDEX IF NOT EXISTS idx_whatsapp_captures_attachment
    ON whatsapp_captures (taxpayer_id, attachment_id) WHERE attachment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_whatsapp_captures_receipt
    ON whatsapp_captures (receipt_capture_id) WHERE receipt_capture_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_whatsapp_captures_task
    ON whatsapp_captures (document_task_id) WHERE document_task_id IS NOT NULL;
