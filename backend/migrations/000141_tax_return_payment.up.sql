-- Tahakkuk ödemesi (#575): verilmiş beyanın tahakkuku ödendi mi, ne zaman,
-- hangi banka hareketiyle (vergi dairesine ödeme). Yalnız verilmiş (filed)
-- beyan ödenmiş işaretlenir. payment_bank_transaction_id mükellefin
-- bank_transactions.id'si; bir ödeme birden çok tahakkuku kapatabilir
-- (KDV ve muhtasar aynı dekontla), bu yüzden tekil değil.
ALTER TABLE tax_returns
    ADD COLUMN paid_on                     DATE        NULL,
    ADD COLUMN payment_bank_transaction_id BIGINT      NULL,
    ADD COLUMN paid_by                     BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN paid_recorded_at            TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_tax_returns_paid
        CHECK (paid_on IS NULL OR status = 'filed'),
    ADD CONSTRAINT chk_tax_returns_payment_tx
        CHECK (payment_bank_transaction_id IS NULL OR paid_on IS NOT NULL);
