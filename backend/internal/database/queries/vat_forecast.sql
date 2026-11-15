-- KDV erken uyarı (issue #33, docs/00 K9 / T10). Kaynak ortak model:
-- invoices + invoice_tax_summaries. Kural (modules/vatforecast):
--   * yalnızca status_normalized = 'active' ve kaynakta silinmemiş faturalar;
--   * tutarlar TL'ye çevrilir: TRY ise 1, değilse invoices.exchange_rate; kuru
--     olmayan döviz faturası dışarıda kalır (VATForecastExcluded sayar);
--   * çevrim özet satırı bazında kuruşa yuvarlanır (ROUND(x * kur, 2)).
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
  AND i.period = @period
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
WHERE i.taxpayer_id = @taxpayer_id AND i.period = @period;

-- name: VATForecastHistory :many
-- Devir zinciri için [from_period, before_period) dönem toplamları:
-- hesaplanan = satış KDV'si − tevkifat, indirilecek = alış KDV'sinin tamamı.
SELECT i.period::text AS period,
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
  AND i.period >= @from_period
  AND i.period < @before_period
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
GROUP BY i.period
ORDER BY i.period;

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
