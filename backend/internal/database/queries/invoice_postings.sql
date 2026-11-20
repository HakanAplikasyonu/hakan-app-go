-- Faturadan muhasebe fişi (#669, Oğuz kararı 20.11).

-- name: InsertInvoiceJournalLink :execrows
-- Bir iş bir faturadan tek fiş üretir; ikinci istek 0 satır döner.
INSERT INTO invoice_journal_links (organization_id, engagement_id, taxpayer_id, invoice_id, journal_draft_id, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @invoice_id, @journal_draft_id, sqlc.narg(created_by))
ON CONFLICT (engagement_id, invoice_id) DO NOTHING;

-- name: GetInvoiceJournalLink :one
SELECT d.uuid, d.status, d.entry_no, d.period
FROM invoice_journal_links l
JOIN journal_drafts d ON d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.invoice_id = @invoice_id;

-- name: ListInvoiceItemsForPosting :many
-- Satırlar ve onaylı hesap kodları (account_assignments).
SELECT it.*,
       COALESCE(acc.account_code, '')::text AS account_code,
       COALESCE(vat.account_code, '')::text AS vat_account_code
FROM invoice_items it
LEFT JOIN account_assignments acc ON acc.organization_id = @organization_id AND acc.engagement_id = @engagement_id
    AND acc.target_type = 'invoice_item' AND acc.target_id = it.id AND acc.field = 'account_code'
LEFT JOIN account_assignments vat ON vat.organization_id = @organization_id AND vat.engagement_id = @engagement_id
    AND vat.target_type = 'invoice_item' AND vat.target_id = it.id AND vat.field = 'vat_account_code'
WHERE it.taxpayer_id = @taxpayer_id AND it.invoice_id = @invoice_id
ORDER BY it.line_no;

-- name: ListUnpostedInvoices :many
-- Fişi olmayan, bu fiş dönemine düşen faturalar: satış belge ayında, alış
-- KDV indirim ayında (#640). Görünürlük ofisin dönemine göre (#8).
SELECT i.id, i.uuid FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
  AND i.status_normalized = 'active'
  AND CASE WHEN i.direction = 'incoming' THEN COALESCE(i.vat_deduction_period, i.period) ELSE i.period END = @period::text
  AND NOT EXISTS (SELECT 1 FROM invoice_journal_links l
                  WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.invoice_id = i.id)
ORDER BY i.issue_date, i.id;

-- name: GetInvoiceCustomerAccount :one
-- Faturanın karşı tarafına onaylanmış cari hesap (account_assignments,
-- target customer); yoksa boş.
SELECT COALESCE((SELECT a.account_code FROM account_assignments a
                 WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
                   AND a.target_type = 'customer' AND a.target_id = @customer_id AND a.field = 'account_code'), '')::text AS account_code;
