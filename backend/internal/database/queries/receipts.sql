-- Fiş yakalama (issue #23).

-- name: ListTaxpayerVehicles :many
SELECT * FROM taxpayer_vehicles WHERE taxpayer_id = @taxpayer_id ORDER BY plate;

-- name: InsertTaxpayerVehicle :one
INSERT INTO taxpayer_vehicles (taxpayer_id, plate, description, valid_from, valid_to, created_by)
VALUES (@taxpayer_id, @plate, @description, sqlc.narg(valid_from), sqlc.narg(valid_to), sqlc.narg(created_by))
RETURNING *;

-- name: DeleteTaxpayerVehicle :execrows
DELETE FROM taxpayer_vehicles WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: VehicleRegisteredOn :one
-- Plaka mükellefin listesinde ve fiş tarihi geçerlilik aralığında mı (T8).
SELECT EXISTS (
    SELECT 1 FROM taxpayer_vehicles
    WHERE taxpayer_id = @taxpayer_id AND plate = @plate
      AND (valid_from IS NULL OR sqlc.narg(on_date)::date IS NULL OR valid_from <= sqlc.narg(on_date)::date)
      AND (valid_to IS NULL OR sqlc.narg(on_date)::date IS NULL OR valid_to >= sqlc.narg(on_date)::date)
)::bool;

-- name: FindExpenseByAttachment :one
SELECT id, uuid FROM expenses WHERE taxpayer_id = @taxpayer_id AND attachment_id = @attachment_id LIMIT 1;

-- name: FindRetailSaleByAttachment :one
SELECT id, uuid FROM retail_sales WHERE taxpayer_id = @taxpayer_id AND attachment_id = @attachment_id LIMIT 1;

-- name: FindDuplicateExpense :one
-- Aynı satıcı + tarih + tutar (+ fiş no varsa) başka görselle gelmiş mi.
SELECT id, uuid FROM expenses
WHERE taxpayer_id = @taxpayer_id AND vendor_tax_number = @vendor_tax_number
  AND expense_date = @expense_date AND amount = @amount
  AND (sqlc.narg(receipt_number)::text IS NULL OR receipt_number IS NULL OR receipt_number = sqlc.narg(receipt_number)::text)
  AND approval_status <> 'rejected'
LIMIT 1;

-- name: FindDuplicateRetailSale :one
SELECT id, uuid FROM retail_sales
WHERE taxpayer_id = @taxpayer_id AND sale_date = @sale_date
  AND (z_report_number = @z_report_number OR total_amount = sqlc.narg(total_amount)::numeric)
  AND approval_status <> 'rejected'
LIMIT 1;

-- name: InsertExpenseDraft :one
INSERT INTO expenses (
    taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, currency, tax_rate,
    tax_exclusive_amount, tax_amount, receipt_type, vendor_tax_number, vendor_name, receipt_number,
    z_report_number, okc_serial_number, vehicle_plate, payment_method, ocr_confidence, ocr_fields,
    attachment_id, raw_payload, content_hash
) VALUES (
    @taxpayer_id, 'receipt_capture', @source_id, 'ocr', sqlc.narg(expense_date), sqlc.narg(period), sqlc.narg(amount), 'TRY',
    sqlc.narg(tax_rate), sqlc.narg(tax_exclusive_amount), sqlc.narg(tax_amount), @receipt_type,
    sqlc.narg(vendor_tax_number), sqlc.narg(vendor_name), sqlc.narg(receipt_number), sqlc.narg(z_report_number),
    sqlc.narg(okc_serial_number), sqlc.narg(vehicle_plate), sqlc.narg(payment_method), @ocr_confidence, @ocr_fields,
    @attachment_id, @raw_payload, @content_hash
)
RETURNING *;

-- name: InsertRetailSaleDraft :one
INSERT INTO retail_sales (
    taxpayer_id, source_system, source_id, capture_method, sale_date, period, z_report_number, okc_serial_number,
    subtotal_amount, tax_amount, total_amount, currency, ocr_confidence, ocr_fields, attachment_id, raw_payload, content_hash
) VALUES (
    @taxpayer_id, 'receipt_capture', @source_id, 'ocr', sqlc.narg(sale_date), sqlc.narg(period), sqlc.narg(z_report_number),
    sqlc.narg(okc_serial_number), sqlc.narg(subtotal_amount), sqlc.narg(tax_amount), sqlc.narg(total_amount), 'TRY',
    @ocr_confidence, @ocr_fields, @attachment_id, @raw_payload, @content_hash
)
RETURNING *;

-- name: GetExpense :one
SELECT * FROM expenses WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: GetRetailSale :one
SELECT * FROM retail_sales WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: UpdateExpenseField :one
-- İnceleme kuyruğu düzeltmesi: tek alan, metin olarak (dönüşüm uygulamada).
UPDATE expenses SET
    expense_date      = CASE WHEN @field::text = 'date' THEN sqlc.narg(date_value)::date ELSE expense_date END,
    period            = CASE WHEN @field::text = 'date' THEN to_char(sqlc.narg(date_value)::date, 'YYYY-MM') ELSE period END,
    amount            = CASE WHEN @field::text = 'total_amount' THEN sqlc.narg(num_value)::numeric ELSE amount END,
    tax_amount        = CASE WHEN @field::text = 'tax_amount' THEN sqlc.narg(num_value)::numeric ELSE tax_amount END,
    tax_rate          = CASE WHEN @field::text = 'tax_rate' THEN sqlc.narg(num_value)::numeric ELSE tax_rate END,
    vendor_tax_number = CASE WHEN @field::text = 'vendor_tax_number' THEN sqlc.narg(text_value)::text ELSE vendor_tax_number END,
    vendor_name       = CASE WHEN @field::text = 'vendor_name' THEN sqlc.narg(text_value)::text ELSE vendor_name END,
    receipt_number    = CASE WHEN @field::text = 'receipt_number' THEN sqlc.narg(text_value)::text ELSE receipt_number END,
    vehicle_plate     = CASE WHEN @field::text = 'vehicle_plate' THEN sqlc.narg(text_value)::text ELSE vehicle_plate END,
    z_report_number   = CASE WHEN @field::text = 'z_report_number' THEN sqlc.narg(text_value)::text ELSE z_report_number END,
    okc_serial_number = CASE WHEN @field::text = 'okc_serial_number' THEN sqlc.narg(text_value)::text ELSE okc_serial_number END,
    payment_method    = CASE WHEN @field::text = 'payment_method' THEN sqlc.narg(text_value)::text ELSE payment_method END,
    tax_exclusive_amount = CASE
        WHEN @field::text IN ('total_amount', 'tax_amount') THEN
            (CASE WHEN @field::text = 'total_amount' THEN sqlc.narg(num_value)::numeric ELSE amount END)
          - COALESCE(CASE WHEN @field::text = 'tax_amount' THEN sqlc.narg(num_value)::numeric ELSE tax_amount END, 0)
        ELSE tax_exclusive_amount END,
    ocr_fields = jsonb_set(ocr_fields, ARRAY[@field::text, 'reviewed'], 'true'::jsonb, true)
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;

-- name: SetExpenseApproval :one
UPDATE expenses SET approval_status = @approval_status, approved_by = sqlc.narg(approved_by), approved_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;

-- name: SetRetailSaleApproval :one
UPDATE retail_sales SET approval_status = @approval_status, approved_by = sqlc.narg(approved_by), approved_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;

-- name: InsertReceiptCapture :one
INSERT INTO receipt_captures (
    organization_id, engagement_id, taxpayer_id, uploaded_by, attachment_id, media_type, kind, status,
    fields, model, expense_id, retail_sale_id, duplicate_of, error, client_key
) VALUES (
    @organization_id, @engagement_id, @taxpayer_id, sqlc.narg(uploaded_by), @attachment_id, @media_type, sqlc.narg(kind), @status,
    @fields, sqlc.narg(model), sqlc.narg(expense_id), sqlc.narg(retail_sale_id), sqlc.narg(duplicate_of), sqlc.narg(error),
    sqlc.narg(client_key)
)
RETURNING *;

-- name: GetReceiptCaptureByClientKey :one
-- Mükellef yüklemesinin tekrarı (#199): aynı Idempotency-Key ilk kaydı döner.
SELECT * FROM receipt_captures
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND taxpayer_id = @taxpayer_id AND client_key = @client_key;

-- name: GetReceiptCapture :one
SELECT * FROM receipt_captures
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListReceiptCaptures :many
SELECT * FROM receipt_captures
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(period)::text IS NULL OR COALESCE((SELECT x.period FROM expenses x WHERE x.taxpayer_id = receipt_captures.taxpayer_id AND x.id = receipt_captures.expense_id), (SELECT y.period FROM retail_sales y WHERE y.taxpayer_id = receipt_captures.taxpayer_id AND y.id = receipt_captures.retail_sale_id), to_char(receipt_captures.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')) = sqlc.narg(period))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: CountReceiptCaptures :one
SELECT count(*)::bigint FROM receipt_captures
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(period)::text IS NULL OR COALESCE((SELECT x.period FROM expenses x WHERE x.taxpayer_id = receipt_captures.taxpayer_id AND x.id = receipt_captures.expense_id), (SELECT y.period FROM retail_sales y WHERE y.taxpayer_id = receipt_captures.taxpayer_id AND y.id = receipt_captures.retail_sale_id), to_char(receipt_captures.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')) = sqlc.narg(period));

-- name: SetReceiptCaptureStatus :one
UPDATE receipt_captures SET status = @status
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: FindCaptureByExpense :one
-- Mükerrer yakalamanın gösterdiği önceki yükleme.
SELECT c.uuid FROM receipt_captures c
JOIN expenses e ON e.taxpayer_id = c.taxpayer_id AND e.id = c.expense_id
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
  AND e.taxpayer_id = @taxpayer_id AND e.id = @expense_id
ORDER BY c.id LIMIT 1;

-- name: UpdateRetailSaleField :one
UPDATE retail_sales SET
    sale_date         = CASE WHEN @field::text = 'date' THEN sqlc.narg(date_value)::date ELSE sale_date END,
    period            = CASE WHEN @field::text = 'date' THEN to_char(sqlc.narg(date_value)::date, 'YYYY-MM') ELSE period END,
    total_amount      = CASE WHEN @field::text = 'total_amount' THEN sqlc.narg(num_value)::numeric ELSE total_amount END,
    tax_amount        = CASE WHEN @field::text = 'tax_amount' THEN sqlc.narg(num_value)::numeric ELSE tax_amount END,
    z_report_number   = CASE WHEN @field::text = 'z_report_number' THEN sqlc.narg(text_value)::text ELSE z_report_number END,
    okc_serial_number = CASE WHEN @field::text = 'okc_serial_number' THEN sqlc.narg(text_value)::text ELSE okc_serial_number END,
    subtotal_amount = CASE
        WHEN @field::text IN ('total_amount', 'tax_amount') THEN
            (CASE WHEN @field::text = 'total_amount' THEN sqlc.narg(num_value)::numeric ELSE total_amount END)
          - COALESCE(CASE WHEN @field::text = 'tax_amount' THEN sqlc.narg(num_value)::numeric ELSE tax_amount END, 0)
        ELSE subtotal_amount END,
    ocr_fields = jsonb_set(ocr_fields, ARRAY[@field::text, 'reviewed'], 'true'::jsonb, true)
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;
