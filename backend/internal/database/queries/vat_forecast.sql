-- KDV erken uyarı (issue #33, docs/00 K9 / T10). Kaynak ortak model:
-- invoices + invoice_tax_summaries. Kural (modules/vatforecast):
--   * yalnızca status_normalized = 'active' ve kaynakta silinmemiş faturalar;
--   * tutarlar TL'ye çevrilir: TRY ise 1, değilse invoices.exchange_rate; kuru
--     olmayan döviz faturası dışarıda kalır (VATForecastExcluded sayar);
--   * çevrim özet satırı bazında kuruşa yuvarlanır (ROUND(x * kur, 2));
--   * dönem KDV dönemidir (#640): alış faturasının KDV indirim dönemi
--     (vat_deduction_period) varsa o ay, yoksa belge ayı.
-- VATForecastLines ile VATForecastHistory aynı formülü kullanır; devir zinciri
-- ekrandaki dönem toplamıyla tutarlı kalır.

-- name: VATForecastLines :many
-- invoice_count satır grubundaki fatura sayısıdır; çok oranlı bir fatura
-- birden çok grupta sayılır (dönem toplamı VATForecastExcluded.included).
SELECT i.direction, i.type, s.tax_rate,
       COALESCE(s.withholding_code, '')::text AS withholding_code,
       COUNT(DISTINCT i.id)::bigint AS invoice_count,
       SUM(ROUND(s.base * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS base,
       SUM(ROUND(s.vat * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS vat,
       SUM(ROUND(s.withholding * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS withholding
FROM invoices i
JOIN invoice_tax_summaries s ON s.invoice_id = i.id AND s.taxpayer_id = i.taxpayer_id
WHERE i.taxpayer_id = @taxpayer_id
  AND (i.period = @period OR i.vat_deduction_period = @period)
  AND COALESCE(i.vat_deduction_period, i.period) = @period
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
GROUP BY i.direction, i.type, s.tax_rate, COALESCE(s.withholding_code, '')
ORDER BY i.direction, s.tax_rate, i.type, COALESCE(s.withholding_code, '');

-- name: VATForecastExcluded :one
-- Tahmine giren fatura sayısı ve girmeyenler (ekranda "şunlar sayılmadı").
SELECT COUNT(*) FILTER (WHERE i.status_normalized = 'active' AND i.deleted_at_source IS NULL
                          AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL))::bigint AS included,
       COUNT(*) FILTER (WHERE i.status_normalized = 'cancelled')::bigint AS cancelled,
       COUNT(*) FILTER (WHERE i.status_normalized = 'draft')::bigint AS draft,
       COUNT(*) FILTER (WHERE i.status_normalized = 'active' AND i.deleted_at_source IS NOT NULL)::bigint AS deleted_at_source,
       COUNT(*) FILTER (WHERE i.status_normalized = 'active' AND i.deleted_at_source IS NULL
                          AND i.currency <> 'TRY' AND i.exchange_rate IS NULL)::bigint AS missing_exchange_rate
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id AND (i.period = @period OR i.vat_deduction_period = @period)
  AND COALESCE(i.vat_deduction_period, i.period) = @period;

-- name: VATForecastHistory :many
-- Devir zinciri için [from_period, before_period) dönem toplamları:
-- hesaplanan = satış KDV'si − tevkifat, indirilecek = alış KDV'sinin tamamı.
SELECT COALESCE(i.vat_deduction_period, i.period)::text AS period,
       COALESCE(SUM(CASE WHEN i.direction = 'outgoing'
           THEN ROUND(s.vat * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)
              - ROUND(s.withholding * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)
           ELSE 0 END), 0)::numeric AS output_vat,
       COALESCE(SUM(CASE WHEN i.direction = 'incoming'
           THEN ROUND(s.vat * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)
           ELSE 0 END), 0)::numeric AS input_vat
FROM invoices i
JOIN invoice_tax_summaries s ON s.invoice_id = i.id AND s.taxpayer_id = i.taxpayer_id
WHERE i.taxpayer_id = @taxpayer_id
  AND COALESCE(i.vat_deduction_period, i.period) >= @from_period::text
  AND COALESCE(i.vat_deduction_period, i.period) < @before_period::text
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
GROUP BY 1
ORDER BY 1;

-- name: GetVATOpeningAtOrBefore :one
-- period'a en yakın (aynı ya da önceki) elle girilmiş açılış devri.
SELECT period::text AS period, amount, note, updated_at
FROM vat_carry_forwards
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period <= @period
ORDER BY period DESC
LIMIT 1;

-- name: ListVATOpenings :many
SELECT period::text AS period, amount, note, updated_at
FROM vat_carry_forwards
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY period;

-- name: UpsertVATOpening :one
INSERT INTO vat_carry_forwards (organization_id, engagement_id, period, amount, note, updated_by)
VALUES (@organization_id, @engagement_id, @period, @amount, sqlc.narg(note), sqlc.narg(updated_by))
ON CONFLICT (organization_id, engagement_id, period)
DO UPDATE SET amount = EXCLUDED.amount, note = EXCLUDED.note, updated_by = EXCLUDED.updated_by
RETURNING period::text AS period, amount, note, updated_at;

-- name: DeleteVATOpening :execrows
DELETE FROM vat_carry_forwards
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period;

-- name: GetVATTrackingSettings :one
-- KDV takibi ofis ayarları (#780); satırı yoksa varsayılan (açık, 29).
SELECT * FROM vat_tracking_settings WHERE organization_id = @organization_id;

-- name: UpsertVATTrackingSettings :one
INSERT INTO vat_tracking_settings (organization_id, report_enabled, report_day, watch_enabled, updated_by, updated_at)
VALUES (@organization_id, @report_enabled, @report_day, @watch_enabled, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET report_enabled = EXCLUDED.report_enabled, report_day = EXCLUDED.report_day,
    watch_enabled = EXCLUDED.watch_enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
