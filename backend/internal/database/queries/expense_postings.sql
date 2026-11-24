-- Masraf kategorileri, masraf listesi ve fiş muhasebeleştirme (issue #384).

-- name: ListExpenseCategories :many
-- Ofis şablonu (engagement_id NULL) ve mükellefe özel satırlar; @engagement_id
-- 0 iken yalnızca şablon.
SELECT * FROM expense_categories
WHERE organization_id = @organization_id
  AND (engagement_id IS NULL OR engagement_id = sqlc.narg(engagement_id)::bigint)
ORDER BY code, engagement_id NULLS FIRST;

-- name: GetExpenseCategory :one
SELECT * FROM expense_categories WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: UpsertExpenseCategory :one
INSERT INTO expense_categories (organization_id, engagement_id, code, name, account_code, vat_account_code,
                                deductible, receipt_types, keywords, is_active, created_by)
VALUES (@organization_id, sqlc.narg(engagement_id), @code, @name, @account_code, @vat_account_code,
        @deductible, @receipt_types::text[], @keywords::text[], @is_active, sqlc.narg(created_by))
ON CONFLICT (organization_id, engagement_id, code) DO UPDATE
SET name = EXCLUDED.name, account_code = EXCLUDED.account_code, vat_account_code = EXCLUDED.vat_account_code,
    deductible = EXCLUDED.deductible, receipt_types = EXCLUDED.receipt_types, keywords = EXCLUDED.keywords,
    is_active = EXCLUDED.is_active
RETURNING *;

-- name: DeleteExpenseCategory :execrows
DELETE FROM expense_categories WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: ListEngagementExpenses :many
-- Ofis panelinde mükellefin masrafları; kategori ve hesap ofis tarafından.
SELECT e.id, e.uuid, e.source_system, e.capture_method, e.expense_date, e.period, e.amount, e.currency, e.tax_rate,
       e.tax_exclusive_amount, e.tax_amount, e.description, e.receipt_type, e.vendor_tax_number, e.vendor_name,
       e.receipt_number, e.vehicle_plate, e.payment_method, e.is_deductible, e.approval_status, e.ocr_confidence,
       e.attachment_id, e.approved_at, e.created_at, e.ocr_fields,
       p.category_code, p.account_code, p.account_source, p.confidence AS account_confidence,
       j.uuid AS journal_uuid, j.entry_no AS journal_entry_no, j.status AS journal_status
FROM expenses e
LEFT JOIN receipt_postings p ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
     AND p.target_type = 'expense' AND p.target_id = e.id
LEFT JOIN journal_drafts j ON j.organization_id = @organization_id AND j.engagement_id = @engagement_id
     AND j.id = p.journal_draft_id
WHERE e.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR e.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL OR e.approval_status = sqlc.narg(status)::text)
  AND (sqlc.narg(category)::text IS NULL OR p.category_code = sqlc.narg(category)::text
       OR (sqlc.narg(category)::text = '-' AND p.category_code IS NULL))
  AND (sqlc.narg(q)::text IS NULL OR e.vendor_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR e.description ILIKE '%' || sqlc.narg(q)::text || '%' OR e.receipt_number = sqlc.narg(q)::text
       OR e.vendor_tax_number = sqlc.narg(q)::text)
ORDER BY e.expense_date DESC NULLS LAST, e.id DESC
LIMIT @lim OFFSET @off;

-- name: SummarizeEngagementExpenses :one
SELECT count(*)::bigint AS total,
       count(*) FILTER (WHERE e.approval_status = 'pending')::bigint AS pending,
       COALESCE(sum(e.amount), 0)::numeric AS amount,
       COALESCE(sum(e.tax_amount), 0)::numeric AS tax_amount
FROM expenses e
LEFT JOIN receipt_postings p ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
     AND p.target_type = 'expense' AND p.target_id = e.id
WHERE e.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR e.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL OR e.approval_status = sqlc.narg(status)::text)
  AND (sqlc.narg(category)::text IS NULL OR p.category_code = sqlc.narg(category)::text
       OR (sqlc.narg(category)::text = '-' AND p.category_code IS NULL))
  AND (sqlc.narg(q)::text IS NULL OR e.vendor_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR e.description ILIKE '%' || sqlc.narg(q)::text || '%' OR e.receipt_number = sqlc.narg(q)::text
       OR e.vendor_tax_number = sqlc.narg(q)::text);

-- name: GetExpenseByUUID :one
SELECT * FROM expenses WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetReceiptPosting :one
SELECT * FROM receipt_postings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND target_type = @target_type AND target_id = @target_id;

-- name: UpsertReceiptPosting :one
INSERT INTO receipt_postings (organization_id, engagement_id, taxpayer_id, target_type, target_id, category_code,
                              account_code, account_source, confidence, decided_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @target_type, @target_id, sqlc.narg(category_code),
        sqlc.narg(account_code), sqlc.narg(account_source), sqlc.narg(confidence), sqlc.narg(decided_by))
ON CONFLICT (engagement_id, target_type, target_id) DO UPDATE
SET category_code = EXCLUDED.category_code, account_code = EXCLUDED.account_code,
    account_source = EXCLUDED.account_source, confidence = EXCLUDED.confidence,
    decided_by = COALESCE(EXCLUDED.decided_by, receipt_postings.decided_by)
RETURNING *;

-- name: SetReceiptPostingJournal :exec
UPDATE receipt_postings SET journal_draft_id = @journal_draft_id
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND target_type = @target_type AND target_id = @target_id;

-- name: VendorExpenseMemory :one
-- Satıcı hafızası: bu satıcının (VKN) son onaylı masrafında kullanılan
-- kategori ve hesap.
SELECT p.category_code, p.account_code
FROM receipt_postings p
JOIN expenses e ON e.taxpayer_id = p.taxpayer_id AND e.id = p.target_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.target_type = 'expense'
  AND e.taxpayer_id = @taxpayer_id AND e.vendor_tax_number = @vendor_tax_number
  AND e.approval_status = 'approved' AND p.account_code IS NOT NULL AND e.id <> @exclude_id
ORDER BY e.approved_at DESC NULLS LAST, e.id DESC
LIMIT 1;

-- name: TaxpayerExpenseMemory :one
-- Mükellef hafızası (#780): satıcının geçmişi yoksa bu mükellefin son onaylı
-- masrafında kullanılan kategori ve hesap.
SELECT p.category_code, p.account_code
FROM receipt_postings p
JOIN expenses e ON e.taxpayer_id = p.taxpayer_id AND e.id = p.target_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.target_type = 'expense'
  AND e.taxpayer_id = @taxpayer_id AND e.approval_status = 'approved' AND p.account_code IS NOT NULL
  AND e.id <> @exclude_id
ORDER BY e.approved_at DESC NULLS LAST, e.id DESC
LIMIT 1;

-- name: InsertExpenseVatLines :execrows
-- Fişten okunan KDV oranı kırılımı (#780).
-- rows: [{"tax_rate","tax_base","tax_amount"}, ...]
INSERT INTO expense_vat_lines (taxpayer_id, expense_id, tax_rate, tax_base, tax_amount)
SELECT @taxpayer_id, @expense_id, r.tax_rate, r.tax_base, r.tax_amount
FROM jsonb_to_recordset(@rows::jsonb) AS r(tax_rate numeric, tax_base numeric, tax_amount numeric)
ON CONFLICT (taxpayer_id, expense_id, tax_rate) DO NOTHING;

-- name: DeleteExpenseVatLines :exec
-- Ön muhasebe programından değişen masrafın kırılımı baştan yazılır (#780).
DELETE FROM expense_vat_lines WHERE taxpayer_id = @taxpayer_id AND expense_id = @expense_id;

-- name: UpsertExpenseVatLine :exec
-- İnceleme kuyruğundan tek oranın kırılımı (mükellefin fiş önizlemesindeki
-- düzeltmesi, Oğuz'un geri bildirimi). Uygulama taslağın beklemede olduğunu
-- önce denetler.
INSERT INTO expense_vat_lines (taxpayer_id, expense_id, tax_rate, tax_base, tax_amount)
VALUES (@taxpayer_id, @expense_id, @tax_rate::numeric, @tax_base::numeric, @tax_amount::numeric)
ON CONFLICT (taxpayer_id, expense_id, tax_rate)
DO UPDATE SET tax_base = EXCLUDED.tax_base, tax_amount = EXCLUDED.tax_amount;

-- name: DeleteExpenseVatLine :exec
DELETE FROM expense_vat_lines
WHERE taxpayer_id = @taxpayer_id AND expense_id = @expense_id AND tax_rate = @tax_rate::numeric;

-- name: MarkExpenseOcrFieldReviewed :exec
-- Kırılım alanı incelendi: ofis ekranındaki "kontrol et" işareti kalkar.
UPDATE expenses SET ocr_fields = jsonb_set(ocr_fields, ARRAY[@field::text, 'reviewed'], 'true'::jsonb, false)
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending';

-- name: ListExpenseVatLines :many
SELECT expense_id, tax_rate, tax_base, tax_amount FROM expense_vat_lines
WHERE taxpayer_id = @taxpayer_id AND expense_id = ANY(@ids::bigint[])
ORDER BY expense_id, tax_rate;

-- name: SetExpenseApprovalByID :one
-- Ofis masraf ekranından onay / ret (yalnızca bekleyen).
UPDATE expenses SET approval_status = @approval_status, approved_by = sqlc.narg(approved_by), approved_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;

-- name: SetCaptureStatusForExpense :exec
-- Masraf ekranından verilen karar fiş yakalama kaydına da yansır.
UPDATE receipt_captures SET status = @status
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND taxpayer_id = @taxpayer_id AND expense_id = @expense_id AND status = 'draft';

-- name: GetJournalDraftByID :one
SELECT * FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: ListPlanAccountsForMains :many
-- Mizan hafızası: etkin planın verilen ana hesaplar altındaki satırları.
SELECT code, name, is_leaf FROM account_plan_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND target = @target
  AND main_code = ANY(@mains::text[])
ORDER BY code;

-- name: InsertImportedExpense :one
-- Excel içe aktarma (#384): satır içeriği kaynak anahtarıdır; aynı satır
-- ikinci kez gelirse eklenmez. Tarihi, satıcısı, VKN'si, fiş numarası ve
-- toplamı aynı bir masraf başka kaynaktan (fiş fotoğrafı, Easytrade) zaten
-- varsa da eklenmez (#453): dışa aktarılan dosya yeniden yüklenince
-- yalnızca yeni satırlar girer.
INSERT INTO expenses (
    taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, currency, tax_rate,
    tax_exclusive_amount, tax_amount, description, receipt_type, vendor_tax_number, vendor_name, receipt_number,
    vehicle_plate, payment_method, content_hash
)
SELECT
    @taxpayer_id, 'excel_import', @source_id, 'excel_import', @expense_date, @period, @amount, 'TRY', sqlc.narg(tax_rate),
    sqlc.narg(tax_exclusive_amount), sqlc.narg(tax_amount), @description, @receipt_type, sqlc.narg(vendor_tax_number)::text,
    sqlc.narg(vendor_name)::text, sqlc.narg(receipt_number)::text, sqlc.narg(vehicle_plate), sqlc.narg(payment_method), @content_hash
WHERE NOT EXISTS (
    SELECT 1 FROM expenses x
    WHERE x.taxpayer_id = @taxpayer_id AND x.expense_date = @expense_date AND x.amount = @amount
      AND x.vendor_tax_number::text IS NOT DISTINCT FROM sqlc.narg(vendor_tax_number)::text
      AND COALESCE(btrim(x.vendor_name), '') = COALESCE(btrim(sqlc.narg(vendor_name)::text), '')
      AND COALESCE(btrim(x.receipt_number), '') = COALESCE(btrim(sqlc.narg(receipt_number)::text), '')
)
ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING
RETURNING *;

-- name: InsertManualExpense :one
-- Ofiste elle girilen masraf (#780): KDV oran kırılımı ayrı satırlarda
-- (expense_vat_lines); onay bekler.
INSERT INTO expenses (
    taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, currency, tax_rate,
    tax_exclusive_amount, tax_amount, description, receipt_type, vendor_tax_number, vendor_name, receipt_number,
    payment_method, content_hash
) VALUES (
    @taxpayer_id, 'manual', @source_id, 'manual', @expense_date, @period, @amount, 'TRY', sqlc.narg(tax_rate),
    sqlc.narg(tax_exclusive_amount), sqlc.narg(tax_amount), @description, @receipt_type, sqlc.narg(vendor_tax_number),
    sqlc.narg(vendor_name), sqlc.narg(receipt_number), sqlc.narg(payment_method), @content_hash
)
RETURNING *;
