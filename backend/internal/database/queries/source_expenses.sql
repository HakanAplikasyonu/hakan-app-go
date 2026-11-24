-- Ön muhasebe programlarından gelen masraflar (#47 Paraşüt): kaynak anahtarıyla
-- tekil (uq_expenses_source); aynı kayıt tekrar gelirse tutarlar güncellenir.

-- name: UpsertSourceExpense :one
INSERT INTO expenses (
    taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, currency, tax_rate,
    tax_exclusive_amount, tax_amount, description, receipt_type, vendor_tax_number, vendor_name, vehicle_plate,
    payment_method, receipt_number, raw_payload, content_hash
) VALUES (
    @taxpayer_id, @source_system, @source_id, 'api', @expense_date, @period, @amount, @currency, sqlc.narg(tax_rate),
    @tax_exclusive_amount, @tax_amount, @description, @receipt_type, sqlc.narg(vendor_tax_number), sqlc.narg(vendor_name),
    sqlc.narg(vehicle_plate), sqlc.narg(payment_method), sqlc.narg(receipt_number), @raw_payload, @content_hash
)
ON CONFLICT (taxpayer_id, source_system, source_id) DO UPDATE
SET expense_date = EXCLUDED.expense_date, period = EXCLUDED.period, amount = EXCLUDED.amount,
    currency = EXCLUDED.currency, tax_rate = EXCLUDED.tax_rate, tax_exclusive_amount = EXCLUDED.tax_exclusive_amount,
    tax_amount = EXCLUDED.tax_amount, description = EXCLUDED.description, vendor_tax_number = EXCLUDED.vendor_tax_number,
    vendor_name = EXCLUDED.vendor_name, vehicle_plate = EXCLUDED.vehicle_plate, payment_method = EXCLUDED.payment_method,
    receipt_number = EXCLUDED.receipt_number, raw_payload = EXCLUDED.raw_payload,
    content_hash = EXCLUDED.content_hash
WHERE expenses.content_hash IS DISTINCT FROM EXCLUDED.content_hash
RETURNING id, (xmax = 0) AS inserted;

-- name: SetSourceConnectionCompany :one
UPDATE source_connections
SET external_company_id = @external_company_id, setup_state = @setup_state, status = @status,
    last_error = NULL, meta = meta || @meta_patch::jsonb
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: SetSourceConnectionState :one
UPDATE source_connections
SET setup_state = @setup_state, status = @status, last_error = sqlc.narg(last_error),
    meta = meta || @meta_patch::jsonb,
    last_success_at = CASE WHEN @mark_success::bool THEN NOW() ELSE last_success_at END
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: ListSourceConnectionsBySystem :many
SELECT * FROM source_connections
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system
ORDER BY id;

-- name: GetExpenseOcrFields :one
-- Fişten oran okuma durumu (#780): kaynak masrafın ocr_fields'ı.
SELECT ocr_fields FROM expenses WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: SetExpenseOcrFields :exec
UPDATE expenses SET ocr_fields = sqlc.narg(ocr_fields)::jsonb
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: ListPendingSourceReceiptReads :many
-- Fişi okunmayı bekleyen kaynak masrafları (#780): yalnız onay bekleyenler
-- (onaylanmış masrafın kırılımı değişmez).
SELECT id, source_id, amount, ocr_fields FROM expenses
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND capture_method = 'api'
  AND approval_status = 'pending' AND ocr_fields -> 'receipt_read' ->> 'status' = 'pending'
ORDER BY id
LIMIT @lim;

-- name: SetSourceReceiptRead :execrows
-- Okuma sonucu yalnız aynı fiş hâlâ bekliyorsa yazılır (arada senkron fişi
-- değiştirdiyse ya da masraf onaylandıysa yazılmaz).
UPDATE expenses SET ocr_fields = @ocr_fields::jsonb
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
  AND ocr_fields -> 'receipt_read' ->> 'key' = @receipt_key::text;

-- name: SetExpenseVatTotals :exec
-- Fişten okunan oranlar yazılınca masrafın KDV toplamı onlardan gelir
-- (#780): KDV = Σ oran KDV'si, matrah = tutar − KDV; birden çok oranda tek
-- oran boş kalır.
UPDATE expenses SET tax_amount = @tax_amount, tax_exclusive_amount = amount - @tax_amount,
    tax_rate = sqlc.narg(tax_rate)
WHERE taxpayer_id = @taxpayer_id AND id = @id;
