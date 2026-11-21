-- POS tahsilatında komisyonun netlenmesi (#565): banka gün sonunda brüt
-- tahsilattan komisyonu düşüp neti yatırır. Müşavir komisyonu ve gider
-- hesabını (653 / 780) girer; fişte banka net borç, komisyon borç, karşı
-- hesap brüt (net + komisyon) alacak olur.
ALTER TABLE bank_transaction_postings
    ADD COLUMN commission_amount       NUMERIC(18,2) NULL,
    ADD COLUMN commission_account_code VARCHAR(64)   NULL,
    ADD CONSTRAINT chk_bank_transaction_postings_commission CHECK (
        (commission_amount IS NULL AND commission_account_code IS NULL)
        OR (commission_amount > 0 AND commission_account_code ~ '^(653|780)(\.[0-9A-Za-z]+)*$')
    );
