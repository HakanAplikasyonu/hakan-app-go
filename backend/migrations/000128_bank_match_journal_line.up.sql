-- Banka hareketi fiş satırıyla da eşleşebilir (#669 G): faturası olmayan
-- tahsilat / ödeme, mevcut bir fişin satırına (cari 120 / 320 alt hesabı,
-- avans, kira …) bağlanır. target_id = journal_draft_lines.id; satır
-- angajman kapsamlıdır, sorgular satırı organization_id + engagement_id ile
-- okur.
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium', 'journal_line'));
