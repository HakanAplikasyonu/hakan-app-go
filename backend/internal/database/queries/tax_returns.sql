-- Beyanname önizlemesi (#390): KDV1, MUHSGK ve Ba/Bs kaynak verisi ile
-- ofisin dönem beyan kayıtları (tax_returns). Fatura kuralları KDV erken
-- uyarıyla aynı (vat_forecast.sql): yalnızca aktif ve kaynakta silinmemiş
-- faturalar; döviz tutarı invoices.exchange_rate ile TL'ye çevrilir ve özet
-- satırı bazında kuruşa yuvarlanır; kuru olmayan döviz faturası dışarıda.
-- KDV1 satırları KDV dönemine göre (#640): alış faturasının KDV indirim
-- dönemi varsa o ay, yoksa belge ayı. Ba/Bs ve MUHSGK belge ayında kalır.

-- name: TaxReturnInvoiceLines :many
SELECT i.direction, i.type, s.tax_rate,
       COALESCE(s.withholding_code, '')::text AS withholding_code,
       COALESCE(NULLIF(s.tax_exemption_code, ''), NULLIF(i.global_tax_exemption_code, ''), '')::text AS exemption_code,
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
GROUP BY i.direction, i.type, s.tax_rate, COALESCE(s.withholding_code, ''),
         COALESCE(NULLIF(s.tax_exemption_code, ''), NULLIF(i.global_tax_exemption_code, ''), '')
ORDER BY i.direction, i.type, s.tax_rate, 4, 5;

-- name: TaxReturnInvoiceStats :one
-- Kontrol listesi: beyana giren / girmeyen fatura sayıları. Sayımlar KDV
-- dönemine göre (#640); moved_in başka aydan bu döneme KDV'si alınan,
-- moved_out bu ayın belgesi olup KDV'si sonraki aya alınan alış faturaları.
SELECT COUNT(*) FILTER (WHERE COALESCE(i.vat_deduction_period, i.period) = @period::text AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL
                          AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL))::bigint AS included,
       COUNT(*) FILTER (WHERE COALESCE(i.vat_deduction_period, i.period) = @period::text AND i.status_normalized = 'draft')::bigint AS draft,
       COUNT(*) FILTER (WHERE COALESCE(i.vat_deduction_period, i.period) = @period::text AND i.status_normalized = 'cancelled')::bigint AS cancelled,
       COUNT(*) FILTER (WHERE COALESCE(i.vat_deduction_period, i.period) = @period::text AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL
                          AND i.currency <> 'TRY' AND i.exchange_rate IS NULL)::bigint AS missing_exchange_rate,
       COUNT(*) FILTER (WHERE COALESCE(i.vat_deduction_period, i.period) = @period::text AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL
                          AND i.tax_summary_source = 'header_estimate')::bigint AS header_estimate,
       COUNT(*) FILTER (WHERE i.vat_deduction_period = @period::text AND i.status_normalized = 'active'
                          AND i.deleted_at_source IS NULL)::bigint AS moved_in,
       COUNT(*) FILTER (WHERE i.period = @period::text AND i.vat_deduction_period IS NOT NULL
                          AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL)::bigint AS moved_out
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id AND (i.period = @period::text OR i.vat_deduction_period = @period::text);

-- name: TaxReturnRetailVATLines :many
-- Onaylı, faturaya bağlanmamış Z raporlarının oran kırılımı (TL).
SELECT v.tax_rate,
       COUNT(DISTINCT r.id)::bigint AS report_count,
       SUM(ROUND(v.tax_base, 2))::numeric AS base,
       SUM(ROUND(v.tax_amount, 2))::numeric AS vat
FROM retail_sales r
JOIN retail_sale_vat_lines v ON v.retail_sale_id = r.id AND v.taxpayer_id = r.taxpayer_id
WHERE r.taxpayer_id = @taxpayer_id
  AND r.period = @period
  AND r.approval_status = 'approved'
  AND r.invoice_id IS NULL
  AND r.currency = 'TRY'
GROUP BY v.tax_rate
ORDER BY v.tax_rate;

-- name: TaxReturnRetailStats :one
-- Z raporları: onaylı / bekleyen sayısı, oran kırılımı olmayan onaylılar ve
-- kredi kartıyla tahsil edilen (KDV dahil) tutar.
SELECT COUNT(*) FILTER (WHERE r.approval_status = 'approved' AND r.invoice_id IS NULL)::bigint AS approved,
       COUNT(*) FILTER (WHERE r.approval_status = 'pending')::bigint AS pending,
       COUNT(*) FILTER (WHERE r.approval_status = 'approved' AND r.invoice_id IS NULL AND NOT EXISTS (
           SELECT 1 FROM retail_sale_vat_lines v WHERE v.retail_sale_id = r.id AND v.taxpayer_id = r.taxpayer_id))::bigint AS without_breakdown,
       COALESCE(SUM(ROUND(r.subtotal_amount, 2)) FILTER (WHERE r.approval_status = 'approved' AND r.invoice_id IS NULL AND NOT EXISTS (
           SELECT 1 FROM retail_sale_vat_lines v WHERE v.retail_sale_id = r.id AND v.taxpayer_id = r.taxpayer_id)), 0)::numeric AS without_breakdown_base,
       COALESCE(SUM(ROUND(r.tax_amount, 2)) FILTER (WHERE r.approval_status = 'approved' AND r.invoice_id IS NULL AND NOT EXISTS (
           SELECT 1 FROM retail_sale_vat_lines v WHERE v.retail_sale_id = r.id AND v.taxpayer_id = r.taxpayer_id)), 0)::numeric AS without_breakdown_vat,
       COALESCE(SUM(ROUND(r.card_amount, 2)) FILTER (WHERE r.approval_status = 'approved' AND r.invoice_id IS NULL), 0)::numeric AS card_total
FROM retail_sales r
WHERE r.taxpayer_id = @taxpayer_id AND r.period = @period AND r.currency = 'TRY';

-- name: TaxReturnExpenseVAT :many
-- Onaylı, KDV'si indirilebilir masraf fişlerinin (ÖKC, akaryakıt, SMM ...)
-- oran kırılımı. Maaş, vergi ödemesi ve banka masrafı KDV taşımaz.
SELECT COALESCE(e.tax_rate, -1)::numeric AS tax_rate,
       COUNT(*)::bigint AS expense_count,
       COALESCE(SUM(ROUND(COALESCE(e.tax_exclusive_amount, e.amount - e.tax_amount, 0), 2)), 0)::numeric AS base,
       COALESCE(SUM(ROUND(e.tax_amount, 2)), 0)::numeric AS vat
FROM expenses e
WHERE e.taxpayer_id = @taxpayer_id
  AND e.period = @period
  AND e.approval_status = 'approved'
  AND e.is_deductible <> 'no'
  AND e.currency = 'TRY'
  AND e.receipt_type NOT IN ('salary', 'tax_payment', 'bank_fee')
  AND COALESCE(e.tax_amount, 0) > 0
GROUP BY COALESCE(e.tax_rate, -1)
ORDER BY 1;

-- name: TaxReturnExpenseStats :one
SELECT COUNT(*) FILTER (WHERE e.approval_status = 'pending')::bigint AS pending,
       COUNT(*) FILTER (WHERE e.approval_status = 'approved' AND e.currency <> 'TRY')::bigint AS foreign_currency,
       COUNT(*) FILTER (WHERE e.approval_status = 'approved' AND e.is_deductible = 'no' AND COALESCE(e.tax_amount, 0) > 0)::bigint AS not_deductible
FROM expenses e
WHERE e.taxpayer_id = @taxpayer_id AND e.period = @period;

-- name: TaxReturnIncomeWithholdings :many
-- MUHSGK ücret dışı stopaj: gelen faturalarda (SMM, müstahsil, kira ...)
-- kesilen gelir vergisi, belge sınıfına göre.
SELECT i.document_class,
       COALESCE(i.income_tax_withholding_rate, 0)::numeric AS rate,
       COUNT(*)::bigint AS invoice_count,
       SUM(ROUND(i.tax_exclusive_amount * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS base,
       SUM(ROUND(i.income_tax_withholding_amount * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS tax
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.period = @period
  AND i.direction = 'incoming'
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
  AND i.income_tax_withholding_amount > 0
GROUP BY i.document_class, COALESCE(i.income_tax_withholding_rate, 0)
ORDER BY i.document_class, 2;

-- name: TaxReturnPayrollLines :many
-- MUHSGK çalışan eki: dönem bordrosunun satırları (TCKN maskeli).
SELECT l.line_no, l.employee_name, l.national_id_masked, e.sgk_no, e.hire_date, e.exit_date, l.days_worked,
       l.gross, l.sgk_base, l.sgk_employee, l.unemployment_employee, l.income_tax_base, l.income_tax,
       l.income_tax_exemption, l.stamp_tax, l.stamp_tax_exemption, l.sgk_employer, l.unemployment_employer, l.sgk_incentive
FROM payroll_lines l
JOIN payroll_runs r ON r.id = l.payroll_run_id AND r.taxpayer_id = l.taxpayer_id
LEFT JOIN employees e ON e.id = l.employee_id AND e.taxpayer_id = l.taxpayer_id
WHERE l.taxpayer_id = @taxpayer_id AND r.period = @period
ORDER BY l.line_no;

-- name: TaxReturnCounterparties :many
-- Ba/Bs: karşı tarafa (VKN / TCKN) göre belge sayısı ve KDV hariç tutar (TL).
SELECT i.direction,
       COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, ''))::text AS counterparty_id,
       COALESCE(MAX(i.customer_name), '')::text AS counterparty_name,
       COUNT(*)::bigint AS document_count,
       SUM(ROUND(i.tax_exclusive_amount * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2))::numeric AS amount
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.period = @period
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
  AND COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, '')) IS NOT NULL
GROUP BY i.direction, 2
ORDER BY i.direction, 5 DESC, 2;

-- name: TaxReturnMissingCounterparty :many
-- Ba/Bs: karşı taraf kimliği olmayan (yabancı ya da eksik) faturalar.
SELECT i.direction,
       COUNT(*)::bigint AS document_count,
       COALESCE(SUM(ROUND(i.tax_exclusive_amount * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)), 0)::numeric AS amount
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.period = @period
  AND i.status_normalized = 'active'
  AND i.deleted_at_source IS NULL
  AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
  AND COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, '')) IS NULL
GROUP BY i.direction
ORDER BY i.direction;

-- name: GetTaxReturn :one
SELECT * FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period;

-- name: ListTaxReturns :many
SELECT * FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(year)::text IS NULL OR period LIKE sqlc.narg(year)::text || '-%')
ORDER BY period DESC, kind;

-- name: UpsertTaxReturnDraft :one
-- Taslağın düzeltme alanları ve onaylanan maddeler; yalnızca taslakken.
INSERT INTO tax_returns (organization_id, engagement_id, kind, period, adjustments, acknowledged, note, created_by)
VALUES (@organization_id, @engagement_id, @kind, @period, @adjustments, @acknowledged, sqlc.narg(note), sqlc.narg(created_by))
ON CONFLICT (organization_id, engagement_id, kind, period) DO UPDATE
SET adjustments = EXCLUDED.adjustments, acknowledged = EXCLUDED.acknowledged, note = EXCLUDED.note
WHERE tax_returns.status = 'draft'
RETURNING *;

-- name: PrepareTaxReturn :one
INSERT INTO tax_returns (organization_id, engagement_id, kind, period, status, adjustments, acknowledged,
                         snapshot, payable_amount, carry_out, prepared_by, prepared_at, created_by)
VALUES (@organization_id, @engagement_id, @kind, @period, 'prepared', @adjustments, @acknowledged,
        @snapshot, @payable_amount, @carry_out, sqlc.narg(prepared_by), NOW(), sqlc.narg(prepared_by))
ON CONFLICT (organization_id, engagement_id, kind, period) DO UPDATE
SET status = 'prepared', adjustments = EXCLUDED.adjustments, acknowledged = EXCLUDED.acknowledged,
    snapshot = EXCLUDED.snapshot, payable_amount = EXCLUDED.payable_amount, carry_out = EXCLUDED.carry_out,
    prepared_by = EXCLUDED.prepared_by, prepared_at = NOW()
WHERE tax_returns.status = 'draft'
RETURNING *;

-- name: ReopenTaxReturn :one
UPDATE tax_returns
SET status = 'draft', snapshot = NULL, payable_amount = NULL, carry_out = NULL, prepared_by = NULL, prepared_at = NULL,
    checked_by = NULL, checked_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period
  AND status = 'prepared'
RETURNING *;

-- name: FileTaxReturn :one
UPDATE tax_returns
SET status = 'filed', filed_on = @filed_on, accrual_number = sqlc.narg(accrual_number),
    note = COALESCE(sqlc.narg(note), note), filed_by = sqlc.narg(filed_by), filed_at = NOW(),
    -- Veren sorumlu kontrol etmiş sayılır (ayrıca işaretlenmediyse).
    checked_by = COALESCE(checked_by, sqlc.narg(filed_by)), checked_at = COALESCE(checked_at, NOW())
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period
  AND status = 'prepared'
RETURNING *;

-- name: CheckTaxReturn :one
-- Son kontrol (#669): hazırlanmış beyanı sorumlu "kontrol edildi" işaretler.
UPDATE tax_returns
SET checked_by = sqlc.narg(checked_by), checked_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period
  AND status = 'prepared'
RETURNING *;

-- name: TaxReturnUserName :one
-- Hazırlayan / kontrol eden / veren kişinin adı (ofis üyesi).
SELECT TRIM(u.name || ' ' || u.surname)::text AS full_name
FROM users u
JOIN organization_members om ON om.user_id = u.id AND om.organization_id = @organization_id
WHERE u.id = @user_id;

-- name: SetTaxReturnAccrualDocument :one
-- Tahakkuk PDF'i (#575) yalnızca verilmiş beyana eklenir; yenisi eskisinin
-- yerine geçer. Dönen satır eski anahtarı taşımaz; eskisini çağıran siler.
UPDATE tax_returns
SET accrual_storage_key = @storage_key::text, accrual_file_name = @file_name::text,
    accrual_size_bytes = @size_bytes::bigint, accrual_sha256 = @sha256::text, accrual_uploaded_by = sqlc.narg(uploaded_by), accrual_uploaded_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period
  AND status = 'filed'
RETURNING *;

-- name: TaxReturnDocumentTaskStats :one
-- Beyan öncesi kontrol (#575): dönemin mükelleften beklenen ve gelip
-- işlenmeyen belgeleri; örnek olarak ilk bekleyen belgenin adı.
SELECT
    COUNT(*) FILTER (WHERE status = 'open')::bigint AS open,
    COUNT(*) FILTER (WHERE status = 'submitted')::bigint AS submitted,
    COALESCE((ARRAY_AGG(COALESCE(NULLIF(title, ''), kind) ORDER BY due_at NULLS LAST, id)
        FILTER (WHERE status = 'open'))[1], '')::text AS first_open
FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period
  AND status IN ('open', 'submitted');

-- name: GetTaxReturnSettings :one
-- Beyan son kontrolü ofis ayarı (#710); satırı yoksa varsayılan (açık).
SELECT * FROM tax_return_settings WHERE organization_id = @organization_id;

-- name: UpsertTaxReturnSettings :one
INSERT INTO tax_return_settings (organization_id, final_check_required, updated_by, updated_at)
VALUES (@organization_id, @final_check_required, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET final_check_required = EXCLUDED.final_check_required, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
