-- Müşavir kontrolleri (issue #31): kuralları besleyen okuma sorguları.

-- name: ListControlInvoices :many
SELECT id, direction, issue_date, payable_amount,
       COALESCE(customer_tax_number, customer_identity_number, lower(btrim(customer_name)), '')::text AS party_key
FROM invoices
WHERE taxpayer_id = @taxpayer_id AND period = @period AND status_normalized <> 'cancelled'
ORDER BY issue_date, id;

-- name: ListControlInvoiceItems :many
SELECT it.id, it.invoice_id, it.name, it.tax_exclusive_amount, i.direction
FROM invoice_items it
JOIN invoices i ON i.taxpayer_id = it.taxpayer_id AND i.id = it.invoice_id
WHERE it.taxpayer_id = @taxpayer_id AND i.taxpayer_id = @taxpayer_id AND i.period = @period
  AND i.status_normalized <> 'cancelled'
ORDER BY it.invoice_id, it.line_no;

-- name: PartyAmountsBefore :many
-- Aynı karşı tarafın, faturadan önceki 12 aydaki tutarları.
SELECT payable_amount FROM invoices
WHERE taxpayer_id = @taxpayer_id AND direction = @direction AND status_normalized <> 'cancelled'
  AND COALESCE(customer_tax_number, customer_identity_number, lower(btrim(customer_name)), '') = @party_key::text
  AND issue_date >= @from_date AND (issue_date < @before_date OR (issue_date = @before_date AND id < @before_id))
ORDER BY issue_date DESC
LIMIT 500;

-- name: TaxpayerAmountsBefore :many
-- Karşı tarafın geçmişi yoksa mükellefin aynı yöndeki son 12 ayı.
SELECT payable_amount FROM invoices
WHERE taxpayer_id = @taxpayer_id AND direction = @direction AND status_normalized <> 'cancelled'
  AND issue_date >= @from_date AND (issue_date < @before_date OR (issue_date = @before_date AND id < @before_id))
ORDER BY issue_date DESC
LIMIT 1000;

-- name: PartySeenBefore :one
SELECT EXISTS (
    SELECT 1 FROM invoices
    WHERE taxpayer_id = @taxpayer_id AND status_normalized <> 'cancelled'
      AND COALESCE(customer_tax_number, customer_identity_number, lower(btrim(customer_name)), '') = @party_key::text
      AND (issue_date < @before_date OR (issue_date = @before_date AND id < @before_id))
)::bool;

-- name: ItemNamesHistory :many
-- Mükellefin dönemden önceki 12 aydaki aynı yönlü kalem adları ve satır sayıları.
SELECT it.name, count(*)::bigint AS lines FROM invoice_items it
JOIN invoices i ON i.taxpayer_id = it.taxpayer_id AND i.id = it.invoice_id
WHERE it.taxpayer_id = @taxpayer_id AND i.taxpayer_id = @taxpayer_id AND i.direction = @direction
  AND i.status_normalized <> 'cancelled'
  AND i.issue_date >= @from_date AND i.issue_date < @before_date AND it.name <> ''
GROUP BY it.name
ORDER BY lines DESC
LIMIT 500;
