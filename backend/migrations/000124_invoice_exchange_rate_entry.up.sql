-- Eksik kuru kim girdi (#571): ofisin elle girdiği kur faturada kimin ve
-- ne zaman girdiğiyle durur; belge panelinde görünür. Kaynaktan gelen kurda
-- ikisi de boştur.
ALTER TABLE invoices
    ADD COLUMN exchange_rate_set_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN exchange_rate_set_at TIMESTAMPTZ NULL;
