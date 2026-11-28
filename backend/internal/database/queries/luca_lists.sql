-- Luca'ya aktar (Excel) listeleri (Oğuz'un geri bildirimi): dönemin gelen /
-- giden faturaları, banka hareketleri ve Z raporları Luca fiş dosyasına.

-- name: ListLucaListInvoices :many
-- Dönemin etkin faturaları, yöne göre; dönem kuralı fatura fişininkiyle
-- aynı: satış belge ayında, alış KDV indirim ayında (#640).
SELECT i.uuid FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
  AND i.status_normalized = 'active'
  AND i.direction = @direction::text
  AND CASE WHEN i.direction = 'incoming' THEN COALESCE(i.vat_deduction_period, i.period) ELSE i.period END = @period::text
ORDER BY i.issue_date, i.id;

-- name: ListLucaListItemSuggestions :many
-- Onaylı kodu olmayan fatura satırının bekleyen hesap önerisi (inceleme
-- kuyruğu): satır Luca dosyasına öneriyle yazılır.
SELECT DISTINCT ON (it.line_no, s.target_field) it.line_no, s.target_field::text AS target_field,
       COALESCE(s.proposed_value->>'account_code', '')::text AS account_code
FROM suggestions s
JOIN invoice_items it ON it.taxpayer_id = @taxpayer_id AND it.id = s.target_id AND it.invoice_id = @invoice_id
WHERE s.organization_id = @organization_id AND s.engagement_id = @engagement_id
  AND s.target_type = 'invoice_item' AND s.target_field IN ('account_code', 'vat_account_code')
  AND s.status = 'pending'
ORDER BY it.line_no, s.target_field, s.created_at DESC;

-- name: ListLucaListBankMovements :many
-- Dönemin banka hareketleri: karşı hesap (onaylı ya da önerilen) ve banka
-- hesabının 102 kodu (bank_ledger_accounts); yoksa boş.
SELECT t.id, t.uuid, t.transaction_date, t.debit_credit, t.amount, t.currency, t.description,
       COALESCE(t.counterparty_name, '')::text AS counterparty_name,
       COALESCE(t.counterparty_tax_number, t.counterparty_identity_number, '')::text AS counterparty_tax_id,
       t.kind_detail,
       COALESCE(p.account_code, '')::text AS account_code,
       COALESCE(l.account_code, '')::text AS bank_account_code
FROM bank_transactions t
LEFT JOIN bank_transaction_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.bank_transaction_id = t.id
LEFT JOIN bank_ledger_accounts l
       ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.cash_box_id = t.cash_box_id
WHERE t.taxpayer_id = @taxpayer_id AND NOT t.is_cancelled AND t.period = @period::text
ORDER BY t.transaction_date, t.id;

-- name: ListLucaListRetailSales :many
-- Dönemin onaylı ve bekleyen Z raporları (reddedilen alınmaz).
SELECT s.id, s.uuid, s.sale_date, COALESCE(s.z_report_number, '')::text AS z_report_number,
       COALESCE(s.okc_serial_number, '')::text AS okc_serial_number,
       s.tax_amount, s.total_amount, s.currency, s.cash_amount, s.card_amount
FROM retail_sales s
WHERE s.taxpayer_id = @taxpayer_id AND s.period = @period::text AND s.approval_status IN ('approved', 'pending')
ORDER BY s.sale_date NULLS LAST, s.id;
