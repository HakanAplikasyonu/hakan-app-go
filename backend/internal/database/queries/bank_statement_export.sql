-- Luca banka ekstresi CSV çıktısı (#465).

-- name: GetCashBoxForExport :one
SELECT id, uuid, name, iban, currency, type FROM cash_boxes
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: ListBankTransactionsForExport :many
-- Bir hesabın tarih aralığındaki iptal edilmemiş hareketleri, ekstre
-- sırasıyla; daha önce kaç kez dışa aktarıldıkları ile.
SELECT t.id, t.uuid, t.transaction_date, t.debit_credit, t.amount, t.currency, t.description, t.description_extra,
       t.receipt_number, t.bank_reference, t.counterparty_name, t.counterparty_iban,
       t.counterparty_tax_number, t.counterparty_identity_number,
       (SELECT COUNT(*) FROM bank_statement_export_items i
        WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id
          AND i.taxpayer_id = t.taxpayer_id AND i.bank_transaction_id = t.id)::int AS previous_exports
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id AND t.cash_box_id = @cash_box_id
  AND t.transaction_date >= @date_from::date AND t.transaction_date <= @date_to::date
  AND NOT t.is_cancelled
ORDER BY t.transaction_date, t.booked_at, t.bank_sequence NULLS LAST, t.id
LIMIT 20000;

-- name: InsertBankStatementExport :one
INSERT INTO bank_statement_exports (organization_id, engagement_id, taxpayer_id, cash_box_id, target, date_from, date_to,
    template_version, template_verified, amount_layout, delimiter, quote, file_name, sha256, size, line_count, repeated_count, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @cash_box_id, @target, @date_from, @date_to,
    @template_version, @template_verified, @amount_layout, @delimiter, @quote, @file_name, @sha256, @size, @line_count, @repeated_count,
    sqlc.narg(created_by))
RETURNING *;

-- name: InsertBankStatementExportItems :execrows
-- Dosyaya giren hareketler. items: [{"id": 1, "previous": 0}, ...]
INSERT INTO bank_statement_export_items (organization_id, engagement_id, bank_statement_export_id, taxpayer_id, bank_transaction_id, previous_exports)
SELECT @organization_id, @engagement_id, @bank_statement_export_id, @taxpayer_id, a.id, a.previous
FROM jsonb_to_recordset(@items::jsonb) AS a(id bigint, previous int);

-- name: ListBankStatementExports :many
SELECT e.*, c.uuid AS cash_box_uuid, c.name AS cash_box_name
FROM bank_statement_exports e
JOIN cash_boxes c ON c.taxpayer_id = e.taxpayer_id AND c.id = e.cash_box_id
WHERE e.organization_id = @organization_id AND e.engagement_id = @engagement_id
ORDER BY e.created_at DESC, e.id DESC
LIMIT @lim OFFSET @off;

-- name: BankTransactionExportMarks :many
-- Banka ekranındaki "dışa aktarıldı" işareti: hareket başına çıktı sayısı
-- ve son çıktı zamanı.
SELECT t.uuid AS bank_transaction_uuid, COUNT(*)::int AS exports, MAX(i.created_at)::timestamptz AS last_exported_at
FROM bank_statement_export_items i
JOIN bank_transactions t ON t.taxpayer_id = i.taxpayer_id AND t.id = i.bank_transaction_id
WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id
  AND t.taxpayer_id = @taxpayer_id AND t.uuid = ANY(@uuids::uuid[])
GROUP BY t.uuid;
