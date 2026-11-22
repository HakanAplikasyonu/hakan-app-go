-- TCMB günlük döviz kurları ve otomatik kur (#780).
--
-- Kur seçimi kuralı (Go tarafında exchangerates.PickRate ile aynı): faturanın
-- tarihinden ÖNCEKİ son TCMB bülteni (tarih < fatura tarihi), en çok 14 gün
-- geriye bakılır (bayram tatilleri). Kur türü ofis ayarı (varsayılan
-- döviz alış); değer / unit = 1 birimin TL karşılığı.

-- name: UpsertExchangeRate :exec
INSERT INTO exchange_rates (date, currency, unit, forex_buying, forex_selling, banknote_buying, banknote_selling, source, fetched_at)
VALUES (@rate_date::date, @currency, @unit, sqlc.narg(forex_buying)::numeric, sqlc.narg(forex_selling)::numeric,
        sqlc.narg(banknote_buying)::numeric, sqlc.narg(banknote_selling)::numeric, @source, NOW())
ON CONFLICT (date, currency) DO UPDATE SET
    unit = EXCLUDED.unit, forex_buying = EXCLUDED.forex_buying, forex_selling = EXCLUDED.forex_selling,
    banknote_buying = EXCLUDED.banknote_buying, banknote_selling = EXCLUDED.banknote_selling,
    source = EXCLUDED.source, fetched_at = NOW();

-- name: ListExchangeRateDates :many
-- Tabloda bülteni olan günler (arşivden eksik gün doldurma).
SELECT DISTINCT date FROM exchange_rates
WHERE date >= @from_date::date AND date <= @to_date::date
ORDER BY date;

-- name: ListExchangeRatesBefore :many
-- Bir dövizin fatura tarihinden önceki son 14 gündeki bültenleri, yeni önce.
SELECT * FROM exchange_rates
WHERE currency = @currency AND date < @before_date::date AND date >= @from_date::date
ORDER BY date DESC
LIMIT 15;

-- name: GetExchangeRateSettings :one
SELECT * FROM exchange_rate_settings WHERE organization_id = @organization_id;

-- name: UpsertExchangeRateSettings :one
INSERT INTO exchange_rate_settings (organization_id, rate_kind, auto_fill, updated_by, updated_at)
VALUES (@organization_id, @rate_kind, @auto_fill, sqlc.narg(updated_by)::bigint, NOW())
ON CONFLICT (organization_id) DO UPDATE SET
    rate_kind = EXCLUDED.rate_kind, auto_fill = EXCLUDED.auto_fill,
    updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;

-- tenant-lint: cross-taxpayer TCMB kur taraması: worker bütün mükelleflerin kuru eksik ya da otomatik kurlu faturalarını rls.System içinde doldurur; ofis ayarı değişince ve fatura kaydında tek ofis / tek fatura süzgeciyle çağrılır
-- tenant-lint: cross-org TCMB kur taraması bütün ofislerde (worker, rls.System); ofis ayarında organization_id süzgeci verilir
-- name: FillInvoiceExchangeRates :execrows
-- Dövizli faturanın kuru yoksa ya da otomatik (tcmb) ise TCMB tablosundan
-- yazılır; elle girilen (manual) ya da kaynakta gelen kura dokunulmaz.
-- Faturanın ofisi mükellefin aktif ilişkisidir; kilitli dönem atlanır.
WITH cand AS (
    SELECT i.id, x.date AS rate_date,
           ROUND((CASE COALESCE(s.rate_kind, 'forex_buying')
                      WHEN 'forex_selling' THEN x.forex_selling
                      WHEN 'banknote_buying' THEN x.banknote_buying
                      WHEN 'banknote_selling' THEN x.banknote_selling
                      ELSE x.forex_buying END) / x.unit, 6)::numeric AS rate
    FROM invoices i
    LEFT JOIN engagements e ON e.taxpayer_id = i.taxpayer_id AND e.ended_at IS NULL
    LEFT JOIN exchange_rate_settings s ON s.organization_id = e.organization_id
    JOIN LATERAL (
        SELECT r.date, r.unit, r.forex_buying, r.forex_selling, r.banknote_buying, r.banknote_selling
        FROM exchange_rates r
        WHERE r.currency = i.currency AND r.date < i.issue_date AND r.date >= i.issue_date - 14
        ORDER BY r.date DESC
        LIMIT 1
    ) x ON TRUE
    WHERE i.currency <> 'TRY' AND i.issue_date IS NOT NULL
      AND (i.exchange_rate IS NULL OR i.exchange_rate = 0 OR i.exchange_rate_source = 'tcmb')
      AND COALESCE(s.auto_fill, TRUE)
      AND (sqlc.narg(organization_id)::bigint IS NULL OR e.organization_id = sqlc.narg(organization_id)::bigint)
      AND (sqlc.narg(invoice_id)::bigint IS NULL OR i.id = sqlc.narg(invoice_id)::bigint)
      AND NOT EXISTS (
          SELECT 1 FROM period_locks pl
          WHERE pl.organization_id = e.organization_id AND pl.engagement_id = e.id AND pl.period = i.period
      )
)
UPDATE invoices inv SET
    exchange_rate = c.rate,
    exchange_rate_source = 'tcmb',
    exchange_rate_date = c.rate_date,
    payable_amount_try = CASE WHEN inv.payable_amount_try IS NULL OR inv.exchange_rate_source = 'tcmb'
                              THEN ROUND(inv.payable_amount * c.rate, 2) ELSE inv.payable_amount_try END
FROM cand c
WHERE inv.id = c.id AND c.rate IS NOT NULL AND c.rate > 0
  AND (inv.exchange_rate IS DISTINCT FROM c.rate OR inv.exchange_rate_date IS DISTINCT FROM c.rate_date
       OR inv.exchange_rate_source IS DISTINCT FROM 'tcmb');

-- tenant-lint: cross-org TCMB kur taraması bütün ofislerde (worker, rls.System); ofis ayarında organization_id süzgeci verilir
-- tenant-lint: org-wide ofis geneli kur türü ayarı ofisin bütün mükelleflerinin taslak yabancı faturalarına uygulanır
-- name: FillForeignInvoiceExchangeRates :execrows
-- Taslak yabancı hizmet faturasının kuru yoksa ya da otomatik (tcmb) ise
-- TCMB tablosundan yazılır; elle girilen kur ve onaylı fatura değişmez.
WITH cand AS (
    SELECT f.id, x.date AS rate_date,
           ROUND((CASE COALESCE(s.rate_kind, 'forex_buying')
                      WHEN 'forex_selling' THEN x.forex_selling
                      WHEN 'banknote_buying' THEN x.banknote_buying
                      WHEN 'banknote_selling' THEN x.banknote_selling
                      ELSE x.forex_buying END) / x.unit, 6)::numeric AS rate
    FROM foreign_service_invoices f
    LEFT JOIN exchange_rate_settings s ON s.organization_id = f.organization_id
    JOIN LATERAL (
        SELECT r.date, r.unit, r.forex_buying, r.forex_selling, r.banknote_buying, r.banknote_selling
        FROM exchange_rates r
        WHERE r.currency = f.currency AND r.date < f.invoice_date AND r.date >= f.invoice_date - 14
        ORDER BY r.date DESC
        LIMIT 1
    ) x ON TRUE
    WHERE f.status = 'draft' AND f.currency <> 'TRY' AND f.invoice_date IS NOT NULL
      AND (f.exchange_rate IS NULL OR f.exchange_rate_source = 'tcmb')
      AND COALESCE(s.auto_fill, TRUE)
      AND (sqlc.narg(organization_id)::bigint IS NULL OR f.organization_id = sqlc.narg(organization_id)::bigint)
)
UPDATE foreign_service_invoices fi SET
    exchange_rate = c.rate, exchange_rate_source = 'tcmb', exchange_rate_date = c.rate_date, updated_at = NOW()
FROM cand c
WHERE fi.id = c.id AND c.rate IS NOT NULL AND c.rate > 0
  AND (fi.exchange_rate IS DISTINCT FROM c.rate OR fi.exchange_rate_date IS DISTINCT FROM c.rate_date
       OR fi.exchange_rate_source IS DISTINCT FROM 'tcmb');
