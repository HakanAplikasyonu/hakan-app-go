-- Kur farkı (#565): döviz hareketi döviz faturasını kapatınca fatura kuru,
-- ödeme günü kuru ve kapanan tutarın TL farkı eşleşmede durur. Fark kâr
-- yönünde artıdır (646 Kambiyo kârları), zarar yönünde eksi (656).
ALTER TABLE bank_transaction_matches
    ADD COLUMN fx_rate_invoice NUMERIC(18,6) NULL,
    ADD COLUMN fx_rate_payment NUMERIC(18,6) NULL,
    ADD COLUMN fx_diff_try     NUMERIC(18,2) NULL,
    ADD CONSTRAINT chk_bank_transaction_matches_fx_rates
        CHECK ((fx_rate_payment IS NULL OR fx_rate_payment > 0) AND (fx_rate_invoice IS NULL OR fx_rate_invoice > 0));
