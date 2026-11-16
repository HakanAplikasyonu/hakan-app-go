-- Müşavir mobil inceleme kuyruğu (issue #239).

-- name: GetSuggestionReviewContext :one
SELECT s.uuid, s.target_type, s.target_id, s.source_entity_type, s.source_entity_id,
       i.uuid AS invoice_uuid, i.invoice_number, i.direction, i.issue_date, i.currency, COALESCE(i.payable_amount::text, '')::text AS payable_amount,
       COALESCE(c.name, '')::text AS counterparty_name,
       it.line_no, it.name AS item_name, COALESCE(it.quantity::text, '')::text AS quantity, it.unit_code,
       COALESCE(it.line_extension_amount::text, '')::text AS line_amount, COALESCE(it.tax_rate::text, '')::text AS tax_rate,
       rc.uuid AS receipt_uuid, rc.media_type AS receipt_media_type
FROM suggestions s
LEFT JOIN invoice_items it ON s.target_type = 'invoice_item' AND it.taxpayer_id = s.taxpayer_id AND it.id = s.target_id
LEFT JOIN invoices i ON i.taxpayer_id = s.taxpayer_id AND i.id = it.invoice_id
LEFT JOIN customers c ON c.taxpayer_id = s.taxpayer_id AND c.id = i.customer_id
LEFT JOIN receipt_captures rc ON s.source_entity_type = 'receipt_capture'
     AND rc.organization_id = s.organization_id AND rc.engagement_id = s.engagement_id AND rc.id = s.source_entity_id
WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id AND s.uuid = @suggestion_uuid;

-- Onaylanan hesap kodu, henüz onaylanmamış yevmiye taslaklarında aynı
-- kaynağa (ör. invoice_item:<id>) bağlı satırlara yazılır (issue #239).
-- name: UpdateDraftJournalLinesAccount :execrows
UPDATE journal_draft_lines l
SET account_code = @account_code
FROM journal_drafts d
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
  AND d.organization_id = l.organization_id AND d.engagement_id = l.engagement_id
  AND d.id = l.journal_draft_id AND d.status = 'draft'
  AND l.source_kind = @source_kind AND l.source_id = @source_id
  AND l.account_code <> @account_code;
