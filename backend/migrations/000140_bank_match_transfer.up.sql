-- Ofis içi virman (#710): bir banka hesabından çıkan hareket aynı mükellefin
-- başka banka hesabına giren hareketine bağlanır. İki satır yazılır (A→B,
-- B→A); target_id = karşı bank_transactions.id (mükellef kapsamlı).
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium', 'journal_line', 'financial_transaction', 'bank_transaction'));
