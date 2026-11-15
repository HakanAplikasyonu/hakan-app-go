-- name: GetInvoiceSourceBySource :one
SELECT * FROM invoice_sources
WHERE taxpayer_id = @taxpayer_id AND source_system = @source_system AND source_id = @source_id;

-- name: UpsertInvoiceSource :one
INSERT INTO invoice_sources (taxpayer_id, invoice_id, source_system, source_id, capture_method, priority, match_rule, content_hash, attachment_id, raw_payload)
VALUES (@taxpayer_id, @invoice_id, @source_system, @source_id, @capture_method, @priority, @match_rule, sqlc.narg(content_hash), sqlc.narg(attachment_id), @raw_payload)
ON CONFLICT (taxpayer_id, source_system, source_id)
DO UPDATE SET last_seen_at = NOW(), content_hash = EXCLUDED.content_hash,
              attachment_id = COALESCE(EXCLUDED.attachment_id, invoice_sources.attachment_id)
RETURNING *;

-- name: ListInvoiceSources :many
SELECT * FROM invoice_sources
WHERE taxpayer_id = @taxpayer_id AND invoice_id = @invoice_id
ORDER BY priority DESC, first_seen_at, id;
