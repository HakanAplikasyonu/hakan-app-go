-- Bakiye uyuşmazlığında mükerrer hareketin hesaptan çıkarılması (#710):
-- ofis bir hareketi "çıkarır" (is_cancelled = true) ya da yeniden dahil
-- eder. Kim / ne zaman burada; bankanın iptal ettiği hareketten (excluded_at
-- boş) bu sayede ayrılır ve yalnız ofisin çıkardığı geri alınabilir.
ALTER TABLE bank_transactions
    ADD COLUMN excluded_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN excluded_at TIMESTAMPTZ NULL;
