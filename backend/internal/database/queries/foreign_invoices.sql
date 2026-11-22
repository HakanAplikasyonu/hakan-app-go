-- Yabancı hizmet faturaları ve KDV2 (#780 A1).

-- name: ListForeignInvoices :many
-- Mükellefin bir dönemdeki yabancı hizmet faturaları; eski önce.
SELECT * FROM foreign_service_invoices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period
ORDER BY invoice_date NULLS LAST, id
LIMIT 500;

-- name: GetForeignInvoice :one
SELECT * FROM foreign_service_invoices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: CreateForeignInvoice :one
INSERT INTO foreign_service_invoices (
    organization_id, engagement_id, period, vendor, country, invoice_no, invoice_date,
    amount, currency, exchange_rate, service_kind, treatment, stopaj_rate, gross_up, vat_rate,
    source_kind, source_uuid, note, created_by
) VALUES (
    @organization_id, @engagement_id, @period, @vendor, @country, @invoice_no, sqlc.narg(invoice_date)::date,
    @amount, @currency, sqlc.narg(exchange_rate)::numeric, @service_kind, @treatment, @stopaj_rate, @gross_up, @vat_rate,
    sqlc.narg(source_kind)::text, sqlc.narg(source_uuid)::uuid, sqlc.narg(note)::text, sqlc.narg(created_by)::bigint
)
RETURNING *;

-- name: UpdateForeignInvoice :one
-- Yalnız taslak değişir; onaylı fatura önce geri alınır.
UPDATE foreign_service_invoices SET
    period = @period, vendor = @vendor, country = @country, invoice_no = @invoice_no,
    invoice_date = sqlc.narg(invoice_date)::date, amount = @amount, currency = @currency,
    exchange_rate = sqlc.narg(exchange_rate)::numeric, service_kind = @service_kind, treatment = @treatment,
    stopaj_rate = @stopaj_rate, gross_up = @gross_up, vat_rate = @vat_rate, note = sqlc.narg(note)::text,
    updated_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'draft'
RETURNING *;

-- name: DeleteForeignInvoice :execrows
DELETE FROM foreign_service_invoices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'draft';

-- name: ConfirmForeignInvoice :one
-- Ofis onayı; kuru eksik fatura onaylanmaz.
UPDATE foreign_service_invoices SET
    status = 'confirmed', confirmed_by = sqlc.narg(confirmed_by)::bigint, confirmed_at = NOW(), updated_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status = 'draft' AND exchange_rate IS NOT NULL
RETURNING *;

-- name: UnconfirmForeignInvoice :one
UPDATE foreign_service_invoices SET
    status = 'draft', confirmed_by = NULL, confirmed_at = NULL, updated_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'confirmed'
RETURNING *;

-- name: ListForeignServiceKindSettings :many
SELECT * FROM foreign_service_kind_settings
WHERE organization_id = @organization_id
ORDER BY service_kind;

-- name: UpsertForeignServiceKindSetting :exec
INSERT INTO foreign_service_kind_settings (
    organization_id, service_kind, treatment, stopaj_rate, gross_up, vat_rate, enabled, updated_by, updated_at
) VALUES (
    @organization_id, @service_kind, @treatment, @stopaj_rate, @gross_up, @vat_rate, @enabled,
    sqlc.narg(updated_by)::bigint, NOW()
)
ON CONFLICT (organization_id, service_kind) DO UPDATE SET
    treatment = EXCLUDED.treatment, stopaj_rate = EXCLUDED.stopaj_rate, gross_up = EXCLUDED.gross_up,
    vat_rate = EXCLUDED.vat_rate, enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW();

-- name: GetForeignInvoiceSettings :one
SELECT * FROM foreign_invoice_settings WHERE organization_id = @organization_id;

-- name: UpsertForeignInvoiceSettings :one
INSERT INTO foreign_invoice_settings (organization_id, enabled, updated_by, updated_at)
VALUES (@organization_id, @enabled, sqlc.narg(updated_by)::bigint, NOW())
ON CONFLICT (organization_id) DO UPDATE SET
    enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;

-- name: TaxReturnForeignInvoices :many
-- KDV2 ve muhtasar önizlemesi (#780 C): dönemin bütün yabancı hizmet
-- faturaları (onaysız ve kursuzlar denetim için sayılır).
SELECT status, treatment, service_kind, amount, currency, exchange_rate, stopaj_rate, gross_up, vat_rate
FROM foreign_service_invoices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period
ORDER BY id;
-- Mükelleften gelen yabancı faturalar (#780 A2).

-- name: GetForeignInvoiceBySource :one
SELECT * FROM foreign_service_invoices
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND source_kind = @source_kind AND source_uuid = @source_uuid;

-- name: CreateForeignInvoiceIntake :one
-- Mükellef yüklemesi / WhatsApp taslağı; satıcı, ülke ve tutar boş olabilir.
-- Aynı kaynak ikinci kez gelirse satır dönmez (GetForeignInvoiceBySource).
INSERT INTO foreign_service_invoices (
    organization_id, engagement_id, period, vendor, country, invoice_no, invoice_date,
    amount, currency, exchange_rate, service_kind, treatment, stopaj_rate, gross_up, vat_rate,
    source_kind, source_uuid, note, prefilled, created_by
) VALUES (
    @organization_id, @engagement_id, @period, @vendor, @country, @invoice_no, sqlc.narg(invoice_date)::date,
    @amount, @currency, sqlc.narg(exchange_rate)::numeric, @service_kind, @treatment, @stopaj_rate, @gross_up, @vat_rate,
    @source_kind::text, @source_uuid::uuid, sqlc.narg(note)::text, @prefilled, sqlc.narg(created_by)::bigint
)
ON CONFLICT (organization_id, engagement_id, source_kind, source_uuid) WHERE source_uuid IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetForeignInvoiceSourceFile :one
-- Faturanın kaynak dosyası (mükellefin attachments deposu).
SELECT a.storage_key, a.content_type, a.file_name
FROM foreign_service_invoices f
JOIN engagements e ON e.organization_id = f.organization_id AND e.id = f.engagement_id
JOIN attachments a ON a.taxpayer_id = e.taxpayer_id AND a.uuid = f.source_uuid
WHERE f.organization_id = @organization_id AND f.engagement_id = @engagement_id AND f.uuid = @uuid;
