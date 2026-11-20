-- KDV indirim dönemi (#640): geç gelen alış faturasının KDV'si, belge tarihi
-- değişmeden sonraki bir ayın beyanında indirilir (KDVK 29/3: vergiyi doğuran
-- olayın olduğu takvim yılını izleyen yılın sonuna kadar). Boş = belge ayı
-- (period). Yalnızca alış faturası, yalnızca ileri bir ay. Kim ve ne zaman
-- değiştirdiği faturada durur; ayrıntı etkinlik kaydında.
ALTER TABLE invoices
    ADD COLUMN vat_deduction_period TEXT        NULL,
    ADD COLUMN vat_deduction_set_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN vat_deduction_set_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_invoices_vat_deduction_period CHECK (
        vat_deduction_period IS NULL OR (
            vat_deduction_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'
            AND direction = 'incoming'
            AND vat_deduction_period > period
            AND left(vat_deduction_period, 4)::int <= left(period, 4)::int + 1
        )
    );
