-- Banka–fatura eşleme (#21). Aday üretme SQL'de, puanlama Go'da (21 §6.2).

-- name: ListBankTransactionsForMatching :many
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND period = @period AND NOT is_cancelled
  AND kind_detail IN ('customer_collection', 'supplier_payment', 'rent_payment', 'utility_payment')
  AND match_status IN ('unmatched', 'suggested')
ORDER BY transaction_date, id;

-- Adaylar: aynı mükellef, aynı yön, aktif, tamamen kapanmamış faturalar;
-- tarih penceresi içinde ve tutar / karşı taraf / fatura no'dan en az biri
-- tutuyor. Tutar, hareketin para biriminde: aynı para birimi ise ödenecek,
-- TRY hesapta dövizli fatura için TRY karşılığı.
-- name: ListInvoiceMatchCandidates :many
SELECT i.id, i.uuid, i.invoice_number, i.issue_date, i.payment_due_date, i.currency,
       i.customer_id, i.customer_name, i.customer_tax_number, i.customer_identity_number,
       COALESCE(c.bank_accounts, '[]'::jsonb) AS bank_accounts,
       (CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END)::numeric AS comparable_amount,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = i.taxpayer_id AND m.target_type = 'invoice' AND m.target_id = i.id), 0)::numeric AS matched_amount
FROM invoices i
LEFT JOIN customers c ON c.taxpayer_id = i.taxpayer_id AND c.id = i.customer_id
WHERE i.taxpayer_id = @taxpayer_id
  AND i.direction = @direction
  AND i.status_normalized = 'active'
  AND i.issue_date BETWEEN @date_from AND @date_to
  AND (i.currency = @currency::text OR (@currency::text = 'TRY' AND i.payable_amount_try IS NOT NULL))
  AND (
        abs((CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END) - @amount::numeric) <= @tolerance::numeric
     OR (sqlc.narg(tax_number)::text IS NOT NULL AND i.customer_tax_number = sqlc.narg(tax_number)::text)
     OR (sqlc.narg(identity_number)::text IS NOT NULL AND i.customer_identity_number = sqlc.narg(identity_number)::text)
     OR i.invoice_number = ANY(@invoice_numbers::text[])
     OR (sqlc.narg(iban)::text IS NOT NULL AND c.bank_accounts @> jsonb_build_array(jsonb_build_object('iban', sqlc.narg(iban)::text)))
  )
ORDER BY i.issue_date DESC, i.id
LIMIT 200;

-- name: InsertBankTransactionMatch :one
INSERT INTO bank_transaction_matches (
    taxpayer_id, bank_transaction_id, target_type, target_id, amount, score, method, evidence, matched_by
) VALUES (
    @taxpayer_id, @bank_transaction_id, @target_type, @target_id, @amount, @score, @method, @evidence, sqlc.narg(matched_by)
)
ON CONFLICT (taxpayer_id, bank_transaction_id, target_type, target_id) DO NOTHING
RETURNING *;

-- name: DeleteBankTransactionMatches :exec
DELETE FROM bank_transaction_matches WHERE taxpayer_id = @taxpayer_id AND bank_transaction_id = @bank_transaction_id;

-- name: ListBankTransactionMatches :many
SELECT * FROM bank_transaction_matches
WHERE taxpayer_id = @taxpayer_id AND bank_transaction_id = @bank_transaction_id
ORDER BY id;

-- name: SetBankTransactionMatchStatus :exec
UPDATE bank_transactions SET match_status = @match_status
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- Dönem kontrolleri (21 §6.3) için dönemin bütün geçerli hareketleri.
-- name: ListBankTransactionsForControls :many
SELECT id, uuid, transaction_date, debit_credit, amount, currency, kind, kind_detail, match_status,
       counterparty_name, counterparty_iban, counterparty_tax_number, counterparty_identity_number,
       payment_purpose, description
FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND period = @period AND NOT is_cancelled AND possible_duplicate_of IS NULL
ORDER BY transaction_date, id;

-- Tek hareketin ekranı (#528): hareket, eşleşmeleri ve adayları.
-- name: GetBankTransactionByUUID :one
SELECT * FROM bank_transactions WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: ListBankTransactionMatchInvoices :many
SELECT m.amount, m.score, m.method, m.matched_at,
       i.uuid AS invoice_uuid, i.invoice_number, i.issue_date, i.customer_name, i.currency, i.payable_amount
FROM bank_transaction_matches m
JOIN invoices i ON i.taxpayer_id = m.taxpayer_id AND i.id = m.target_id
WHERE m.taxpayer_id = @taxpayer_id AND m.bank_transaction_id = @bank_transaction_id AND m.target_type = 'invoice'
ORDER BY i.issue_date, i.id;

-- Elle eşleştirmede seçilen faturalar: aynı yön, aktif; açık tutar bu
-- hareketin eski eşleşmeleri hariç (yeniden eşleştirme).
-- name: ListInvoicesForManualMatch :many
SELECT i.id, i.uuid, i.invoice_number,
       (CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END)::numeric AS comparable_amount,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = i.taxpayer_id AND m.target_type = 'invoice' AND m.target_id = i.id
                   AND m.bank_transaction_id <> @bank_transaction_id), 0)::numeric AS matched_amount
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.uuid = ANY(@uuids::uuid[])
  AND i.direction = @direction
  AND i.status_normalized = 'active'
  AND (i.currency = @currency::text OR (@currency::text = 'TRY' AND i.payable_amount_try IS NOT NULL))
ORDER BY i.issue_date, i.id;
