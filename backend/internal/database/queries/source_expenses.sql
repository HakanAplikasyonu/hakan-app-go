-- Ön muhasebe programlarından gelen masraflar (#47 Paraşüt): kaynak anahtarıyla
-- tekil (uq_expenses_source); aynı kayıt tekrar gelirse tutarlar güncellenir.

-- name: UpsertSourceExpense :one
INSERT INTO expenses (
    taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, currency, tax_rate,
    tax_exclusive_amount, tax_amount, description, receipt_type, vendor_tax_number, vendor_name, vehicle_plate,
    raw_payload, content_hash
) VALUES (
    @taxpayer_id, @source_system, @source_id, 'api', @expense_date, @period, @amount, @currency, sqlc.narg(tax_rate),
    @tax_exclusive_amount, @tax_amount, @description, @receipt_type, sqlc.narg(vendor_tax_number), sqlc.narg(vendor_name),
    sqlc.narg(vehicle_plate), @raw_payload, @content_hash
)
ON CONFLICT (taxpayer_id, source_system, source_id) DO UPDATE
SET expense_date = EXCLUDED.expense_date, period = EXCLUDED.period, amount = EXCLUDED.amount,
    currency = EXCLUDED.currency, tax_rate = EXCLUDED.tax_rate, tax_exclusive_amount = EXCLUDED.tax_exclusive_amount,
    tax_amount = EXCLUDED.tax_amount, description = EXCLUDED.description, vendor_tax_number = EXCLUDED.vendor_tax_number,
    vendor_name = EXCLUDED.vendor_name, vehicle_plate = EXCLUDED.vehicle_plate, raw_payload = EXCLUDED.raw_payload,
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
