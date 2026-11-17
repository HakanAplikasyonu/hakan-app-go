-- Mükellef raporu (issue #386): dönem (ay / yıl) ciro, gider, KDV ve
-- gelir-gider trendi. Tutarlar TL'ye çevrilir (döviz faturada kur ile).
-- Yalnızca mükellefin kendi verisi; kapsam taxpayer_id ile.

-- name: ReportInvoiceMonthly :many
-- Aktif faturalar, ay ve yöne göre; iade faturaları ayrı satırda (is_return):
-- gelen iade satışı, giden iade alışı azaltır.
SELECT period::text AS period, direction, (type = 'IADE')::boolean AS is_return,
       count(*)::bigint AS invoices,
       coalesce(sum(CASE WHEN currency = 'TRY' THEN tax_exclusive_amount
                         ELSE tax_exclusive_amount * coalesce(exchange_rate, 1) END), 0)::numeric AS base,
       coalesce(sum(CASE WHEN currency = 'TRY' THEN tax_amount
                         ELSE tax_amount * coalesce(exchange_rate, 1) END), 0)::numeric AS vat
FROM invoices
WHERE taxpayer_id = @taxpayer_id AND period >= @from_period::text AND period <= @to_period::text
  AND status_normalized = 'active'
GROUP BY period, direction, (type = 'IADE')
ORDER BY period, direction;

-- name: ReportExpenseMonthly :many
-- Onaylı masraflar (fiş, makbuz), ay bazında. KDV hariç tutar yoksa toplam.
SELECT period::text AS period, count(*)::bigint AS expenses,
       coalesce(sum(coalesce(tax_exclusive_amount, amount - coalesce(tax_amount, 0), amount)), 0)::numeric AS base,
       coalesce(sum(coalesce(tax_amount, 0)), 0)::numeric AS vat
FROM expenses
WHERE taxpayer_id = @taxpayer_id AND period >= @from_period::text AND period <= @to_period::text
  AND approval_status = 'approved' AND period IS NOT NULL
GROUP BY period
ORDER BY period;

-- name: ReportRetailMonthly :many
-- Onaylı Z raporları (perakende satış), faturaya bağlanmamış olanlar
-- (bağlı olan faturada zaten sayılır).
SELECT period::text AS period, count(*)::bigint AS reports,
       coalesce(sum(coalesce(subtotal_amount, total_amount - coalesce(tax_amount, 0))), 0)::numeric AS base,
       coalesce(sum(coalesce(tax_amount, 0)), 0)::numeric AS vat
FROM retail_sales
WHERE taxpayer_id = @taxpayer_id AND period >= @from_period::text AND period <= @to_period::text
  AND approval_status = 'approved' AND invoice_id IS NULL AND period IS NOT NULL
GROUP BY period
ORDER BY period;

-- name: ReportExpenseCategories :many
-- Gider kategorileri: onaylı masraflar belge türüne göre. Masraf
-- kategorileri gelince (masraf şeridi) anahtar o sütundan okunacak; usecase
-- bilinmeyen anahtarı da gösterir.
SELECT receipt_type::text AS category, count(*)::bigint AS records,
       coalesce(sum(coalesce(tax_exclusive_amount, amount - coalesce(tax_amount, 0), amount)), 0)::numeric AS base
FROM expenses
WHERE taxpayer_id = @taxpayer_id AND period >= @from_period::text AND period <= @to_period::text
  AND approval_status = 'approved' AND period IS NOT NULL
GROUP BY receipt_type
ORDER BY base DESC;

-- name: ReportLatestSummary :one
-- Dönemin AI yorumu: onaylı özet öncelikli, yoksa son taslak.
SELECT uuid, period::text AS period, status, body, approved_at
FROM period_summaries
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND period = @period AND status IN ('approved', 'draft')
ORDER BY (status = 'approved') DESC, created_at DESC
LIMIT 1;

-- name: ReportTaxpayer :one
-- Rapor başlığı ve gönderim adresi.
SELECT title, tax_id, contact_email FROM taxpayers WHERE id = @taxpayer_id;
