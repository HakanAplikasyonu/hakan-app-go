-- Çift kayıt koruması (#466): Luca çekim ayarları, fiş satırlarının kaynak
-- belgeleri ve belge → dosya izi.

-- name: GetOfficeExportSettings :one
SELECT * FROM office_export_settings
WHERE organization_id = @organization_id;

-- name: UpsertOfficeExportSettings :one
INSERT INTO office_export_settings (organization_id, luca_pull_integrators, updated_by)
VALUES (@organization_id, @luca_pull_integrators::text[], sqlc.narg(updated_by))
ON CONFLICT (organization_id) DO UPDATE SET
    luca_pull_integrators = EXCLUDED.luca_pull_integrators,
    updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: ResolveInvoiceLineSources :many
-- Fiş satırlarının kaynak faturası: satırın source_id'si fatura (invoice) ya
-- da kalem (invoice_item) kimliğidir; ETTN, numara, tarih, kaynak sistem ve
-- karşı tarafın VKN / TCKN'si (Luca: TC/Vergi No) faturadan gelir.
SELECT src.source_kind::text AS source_kind, src.source_id::text AS source_id,
       i.uuid, i.ettn, i.invoice_number::text AS invoice_number, i.issue_date, i.source_system,
       COALESCE(i.customer_tax_number, i.customer_identity_number, '')::text AS counterparty_tax_id
FROM (
    SELECT 'invoice'::text AS source_kind, iv.id::text AS source_id, iv.id AS invoice_id
    FROM invoices iv
    WHERE iv.taxpayer_id = @taxpayer_id AND iv.id = ANY(@invoice_ids::bigint[])
    UNION ALL
    SELECT 'invoice_item'::text, it.id::text, it.invoice_id
    FROM invoice_items it
    WHERE it.taxpayer_id = @taxpayer_id AND it.id = ANY(@item_ids::bigint[])
) src
JOIN invoices i ON i.taxpayer_id = @taxpayer_id AND i.id = src.invoice_id;

-- name: TaxpayerEntegratorSystems :many
-- Mükellefin entegratör bağlantıları (entegrator_<ad>): kaynağı entegratör
-- olmayan bir faturanın (Excel, UBL yükleme) hangi entegratörden kesildiğini
-- tahmin etmek için.
SELECT DISTINCT source_system FROM source_connections
WHERE taxpayer_id = @taxpayer_id AND source_system LIKE 'entegrator\_%';

-- name: ListLiveDocumentExports :many
-- Aynı programa canlı (geri alınmamış) bir çıktıda gitmiş belgeler.
SELECT d.document_key, d.file_name, b.uuid AS batch_uuid, b.created_at,
       jd.uuid AS journal_draft_uuid, jd.period, jd.entry_no
FROM export_batch_documents d
JOIN export_batches b
  ON b.organization_id = d.organization_id AND b.engagement_id = d.engagement_id AND b.id = d.export_batch_id
JOIN export_batch_items it
  ON it.organization_id = d.organization_id AND it.engagement_id = d.engagement_id
 AND it.export_batch_id = d.export_batch_id AND it.journal_draft_id = d.journal_draft_id
JOIN journal_drafts jd
  ON jd.organization_id = d.organization_id AND jd.engagement_id = d.engagement_id AND jd.id = d.journal_draft_id
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND b.driver = @driver
  AND it.reversed_at IS NULL
  AND d.document_key = ANY(@document_keys::text[])
ORDER BY b.created_at, d.id;

-- name: InsertExportBatchDocument :exec
INSERT INTO export_batch_documents (organization_id, engagement_id, export_batch_id, journal_draft_id, source_kind,
    document_key, invoice_uuid, document_no, document_date, file_name, confirmed_codes, confirmed_by, confirmed_at)
VALUES (@organization_id, @engagement_id, @export_batch_id, @journal_draft_id, @source_kind,
    @document_key, sqlc.narg(invoice_uuid), sqlc.narg(document_no), sqlc.narg(document_date), @file_name,
    @confirmed_codes::text[], sqlc.narg(confirmed_by),
    CASE WHEN cardinality(@confirmed_codes::text[]) > 0 THEN NOW() END);

-- name: ListExportDocuments :many
-- Dışa aktarım geçmişinde belge → dosya izi. batch_id verilirse o çıktının
-- belgeleri; q verilirse ETTN ya da belge numarasında arama.
SELECT d.id, d.document_key, d.invoice_uuid, d.document_no, d.document_date, d.file_name,
       d.confirmed_codes, d.confirmed_at, d.created_at,
       b.uuid AS batch_uuid, b.driver,
       jd.uuid AS journal_draft_uuid, jd.period, jd.entry_no,
       it.reversed_at
FROM export_batch_documents d
JOIN export_batches b
  ON b.organization_id = d.organization_id AND b.engagement_id = d.engagement_id AND b.id = d.export_batch_id
JOIN journal_drafts jd
  ON jd.organization_id = d.organization_id AND jd.engagement_id = d.engagement_id AND jd.id = d.journal_draft_id
LEFT JOIN export_batch_items it
  ON it.organization_id = d.organization_id AND it.engagement_id = d.engagement_id
 AND it.export_batch_id = d.export_batch_id AND it.journal_draft_id = d.journal_draft_id
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND (sqlc.narg(batch_id)::bigint IS NULL OR d.export_batch_id = sqlc.narg(batch_id))
  AND (sqlc.narg(q)::text IS NULL
       OR d.document_key LIKE '%' || lower(sqlc.narg(q)) || '%'
       OR d.document_no ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY d.created_at DESC, d.id DESC
LIMIT @lim OFFSET @off;
