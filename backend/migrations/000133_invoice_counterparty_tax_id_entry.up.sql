-- Geçersiz / eksik VKN'nin ofiste tamamlanması (#669, INVALID_TAX_ID): karşı
-- tarafın numarası kaynakta yoksa ya da geçersizse ofis girer. Kim / ne
-- zaman burada; cari bağlanır, uyarı çözülmüş olur.
ALTER TABLE invoices
    ADD COLUMN counterparty_tax_id_set_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN counterparty_tax_id_set_at TIMESTAMPTZ NULL;
