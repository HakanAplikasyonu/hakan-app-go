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
WHERE receipt_captures.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR COALESCE((SELECT x.period FROM expenses x WHERE x.taxpayer_id = receipt_captures.taxpayer_id AND x.id = receipt_captures.expense_id), (SELECT y.period FROM retail_sales y WHERE y.taxpayer_id = receipt_captures.taxpayer_id AND y.id = receipt_captures.retail_sale_id), to_char(receipt_captures.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')) = sqlc.narg(period))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org mükellef panelinde kendi fişleri; taxpayer_id ile sınırlı
-- name: CountTaxpayerReceiptCaptures :one
SELECT COUNT(*)::bigint FROM receipt_captures WHERE receipt_captures.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR COALESCE((SELECT x.period FROM expenses x WHERE x.taxpayer_id = receipt_captures.taxpayer_id AND x.id = receipt_captures.expense_id), (SELECT y.period FROM retail_sales y WHERE y.taxpayer_id = receipt_captures.taxpayer_id AND y.id = receipt_captures.retail_sale_id), to_char(receipt_captures.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')) = sqlc.narg(period));

-- tenant-lint: cross-org mükellef panelinde kendi fişinin fotoğrafı (mobil belge görüntüleyici); taxpayer_id ile sınırlı
-- name: GetTaxpayerReceiptImage :one
SELECT a.storage_key, a.content_type, a.size_bytes
FROM receipt_captures r
JOIN attachments a ON a.taxpayer_id = r.taxpayer_id AND a.id = r.attachment_id
WHERE r.taxpayer_id = @taxpayer_id AND a.taxpayer_id = @taxpayer_id AND r.uuid = @receipt_uuid;

-- tenant-lint: cross-org mükellef panelinde kullanıcının kendi yüklemeleri; taxpayer_id + actor_id ile sınırlı
-- name: ListTaxpayerUploadJobs :many
SELECT * FROM ubl_upload_jobs
WHERE taxpayer_id = @taxpayer_id AND actor_id = @actor_id
  AND (sqlc.narg(period)::text IS NULL OR to_char(created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM') = sqlc.narg(period))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org mükellef panelinde kullanıcının kendi yüklemesi; taxpayer_id + actor_id ile sınırlı
-- name: GetTaxpayerUploadJob :one
SELECT * FROM ubl_upload_jobs
WHERE taxpayer_id = @taxpayer_id AND actor_id = @actor_id AND uuid = @uuid;

-- name: TaxpayerSalesMonthly :many
-- Mükellef panelinde satış raporu (#780): aktif giden faturalar, ay bazında.
-- İade (IADE) faturaları satıştan düşülür; döviz faturada kur ile TL.
SELECT period::text AS period,
       (count(*) FILTER (WHERE type <> 'IADE'))::bigint AS invoices,
       coalesce(sum(CASE WHEN type = 'IADE' THEN -1 ELSE 1 END *
                    CASE WHEN currency = 'TRY' THEN tax_exclusive_amount
                         ELSE tax_exclusive_amount * coalesce(exchange_rate, 1) END), 0)::numeric AS base,
       coalesce(sum(CASE WHEN type = 'IADE' THEN -1 ELSE 1 END *
                    CASE WHEN currency = 'TRY' THEN tax_amount
                         ELSE tax_amount * coalesce(exchange_rate, 1) END), 0)::numeric AS vat
FROM invoices
WHERE taxpayer_id = @taxpayer_id AND direction = 'outgoing' AND status_normalized = 'active'
  AND period >= @from_period::text AND period <= @to_period::text
GROUP BY period
ORDER BY period;

-- name: TaxpayerSalesTopBuyers :many
-- Dönemin en çok satış yapılan alıcıları (#780): aktif giden, iade hariç
-- faturalar; alıcı VKN/TCKN'ye göre, kimliksiz alıcılar unvana göre.
SELECT coalesce(max(customer_name), '')::text AS name,
       coalesce(nullif(customer_tax_number, ''), nullif(customer_identity_number, ''), '')::text AS tax_id,
       count(*)::bigint AS invoices,
       coalesce(sum(CASE WHEN currency = 'TRY' THEN tax_exclusive_amount
                         ELSE tax_exclusive_amount * coalesce(exchange_rate, 1) END), 0)::numeric AS base
FROM invoices
WHERE taxpayer_id = @taxpayer_id AND direction = 'outgoing' AND status_normalized = 'active'
  AND type <> 'IADE' AND period = @period::text
GROUP BY coalesce(nullif(customer_tax_number, ''), nullif(customer_identity_number, ''), ''),
         CASE WHEN coalesce(nullif(customer_tax_number, ''), nullif(customer_identity_number, '')) IS NULL
              THEN coalesce(customer_name, '') ELSE '' END
ORDER BY base DESC, name
LIMIT @lim;

-- Gönderdiklerim (#780 Senaryo B): mükellefin telefondan gönderdiği fişler
-- ve belge isteği yüklemeleri, her birinin ofisteki durumu ve hangi ofise
-- ulaştığı. Ret gerekçesi belge isteğinde ofisin notudur (yeniden istendi)
-- ya da iptal gerekçesidir; fişte ayrı gerekçe tutulmaz.

-- tenant-lint: cross-org mükellef uygulamasında kendi gönderdiği fişler; taxpayer_id ile sınırlı
-- name: ListTaxpayerSubmittedReceipts :many
SELECT c.uuid, c.kind, c.status, c.media_type, c.error, c.created_at, c.updated_at,
       (c.expense_id IS NOT NULL OR c.retail_sale_id IS NOT NULL)::bool AS has_draft,
       COALESCE(c.fields->'vendor_name'->>'value', '')::text AS vendor_name,
       COALESCE(c.fields->'total_amount'->>'value', '')::text AS total_amount,
       o.name::text AS office_name
FROM receipt_captures c
JOIN organizations o ON o.id = c.organization_id
WHERE c.taxpayer_id = @taxpayer_id
ORDER BY c.created_at DESC, c.id DESC
LIMIT @lim;

-- tenant-lint: cross-org mükellef uygulamasında kendi belge isteği yüklemeleri; taxpayer_id ile sınırlı
-- name: ListTaxpayerSubmittedTaskUploads :many
SELECT u.uuid, u.created_at, t.uuid AS task_uuid, t.title, t.kind AS task_kind, t.status AS task_status,
       t.note, COALESCE(t.cancel_reason, '')::text AS cancel_reason, t.updated_at AS task_updated_at,
       COALESCE(a.content_type, '')::text AS content_type, COALESCE(a.file_name, '')::text AS file_name,
       o.name::text AS office_name
FROM document_task_uploads u
JOIN document_tasks t ON t.organization_id = u.organization_id AND t.id = u.task_id AND t.taxpayer_id = u.taxpayer_id
JOIN attachments a ON a.taxpayer_id = u.taxpayer_id AND a.id = u.attachment_id
JOIN organizations o ON o.id = u.organization_id
WHERE u.taxpayer_id = @taxpayer_id
ORDER BY u.created_at DESC, u.id DESC
LIMIT @lim;

-- tenant-lint: cross-org mükellefin bugünkü ofisinin adı (gönderim onayı ekranı); taxpayer_id ile sınırlı
-- name: GetTaxpayerOfficeName :one
SELECT o.name::text AS office_name
FROM engagements e
JOIN organizations o ON o.id = e.organization_id
WHERE e.taxpayer_id = @taxpayer_id AND e.ended_at IS NULL
ORDER BY e.id DESC
LIMIT 1;
