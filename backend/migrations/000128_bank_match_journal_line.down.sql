-- Fiş satırı eşleşmeleri silinir (veri kaybı: bu eşleşmeler geri gelmez;
-- hareketin match_status'u değişmez, dönem eşleştirmesi yeniden çalıştırılır).
DELETE FROM bank_transaction_matches WHERE target_type = 'journal_line';
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium'));
