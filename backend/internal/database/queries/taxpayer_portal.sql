-- Mükellef web paneli (issue #39). Her sorgu X-Taxpayer ile çözülen
-- mükellefin taxpayer_id'siyle sınırlıdır; ofis bağlamı (organization_id)
-- burada yoktur.

-- name: ListTaxpayerInvoices :many
SELECT uuid, ettn, invoice_number, direction, type, profile, issue_date, period,
       customer_name, customer_tax_number, customer_identity_number, currency,
       tax_exclusive_amount, tax_amount, withholding_amount, payable_amount,
       status_normalized, source_system, capture_method, id
FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period))
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction))
ORDER BY issue_date DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: ListTaxpayerInvoicesAfter :many
-- İmleçli sayfa (#182), bkz. ListEngagementInvoicesAfter.
SELECT uuid, ettn, invoice_number, direction, type, profile, issue_date, period,
       customer_name, customer_tax_number, customer_identity_number, currency,
       tax_exclusive_amount, tax_amount, withholding_amount, payable_amount,
       status_normalized, source_system, capture_method, id
FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND (issue_date, id) < (@cursor_date::date, @cursor_id::bigint)
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period))
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction))
ORDER BY issue_date DESC, id DESC
LIMIT @lim;

-- name: CountTaxpayerInvoices :one
SELECT COUNT(*)::bigint FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period))
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction));

-- tenant-lint: cross-org mükellef panelinde kendi fişleri; taxpayer_id ile sınırlı, ofis bağlamı yok
-- name: ListTaxpayerReceiptCaptures :many
SELECT uuid, kind, status, media_type, fields, error, created_at
FROM receipt_captures
WHERE taxpayer_id = @taxpayer_id
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org mükellef panelinde kendi fişleri; taxpayer_id ile sınırlı
-- name: CountTaxpayerReceiptCaptures :one
SELECT COUNT(*)::bigint FROM receipt_captures WHERE taxpayer_id = @taxpayer_id;

-- tenant-lint: cross-org mükellef panelinde kullanıcının kendi yüklemeleri; taxpayer_id + actor_id ile sınırlı
-- name: ListTaxpayerUploadJobs :many
SELECT * FROM ubl_upload_jobs
WHERE taxpayer_id = @taxpayer_id AND actor_id = @actor_id
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org mükellef panelinde kullanıcının kendi yüklemesi; taxpayer_id + actor_id ile sınırlı
-- name: GetTaxpayerUploadJob :one
SELECT * FROM ubl_upload_jobs
WHERE taxpayer_id = @taxpayer_id AND actor_id = @actor_id AND uuid = @uuid;
