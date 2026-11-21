-- Banka hareketi kaynak finansal işlemine bağlanır (#565): entegrasyondan
-- (Easytrade, KolayBi, Paraşüt) gelen çek, senet ya da kredi kartı ödemesi
-- bankadan geçtiğinde hareket o işleme bağlanır.
-- target_id = source_financial_transactions.id (mükellef kapsamlı).
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium', 'journal_line', 'financial_transaction'));
