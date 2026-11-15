-- Banka hareketi modeli (#20). Bütün sorgular mükellef kapsamında (taxpayer_id).

-- name: InsertCashBox :one
INSERT INTO cash_boxes (
    taxpayer_id, type, name, bank_id, iban, currency, bank_account_number,
    bank_branch_code, bank_account_suffix, bank_customer_no, bank_account_kind,
    bank_connection_id, bank_sync_from
) VALUES (
    @taxpayer_id, @type, @name, sqlc.narg(bank_id), sqlc.narg(iban), @currency, sqlc.narg(bank_account_number),
    sqlc.narg(bank_branch_code), sqlc.narg(bank_account_suffix), sqlc.narg(bank_customer_no), sqlc.narg(bank_account_kind),
    sqlc.narg(bank_connection_id), sqlc.narg(bank_sync_from)
)
RETURNING *;

-- name: GetCashBox :one
SELECT * FROM cash_boxes WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: FindCashBoxByIBAN :one
SELECT * FROM cash_boxes WHERE taxpayer_id = @taxpayer_id AND iban = @iban;

-- name: ListCashBoxes :many
SELECT * FROM cash_boxes WHERE taxpayer_id = @taxpayer_id ORDER BY id;

-- name: FindBankTransactionBySource :one
SELECT bank_transaction_id FROM bank_transaction_sources
WHERE taxpayer_id = @taxpayer_id AND cash_box_id = @cash_box_id
  AND source_system = @source_system AND source_id = @source_id;

-- 21 §5 kural 2: aynı hesap + tarih + yön + tutar + işlem sonrası bakiye.
-- name: FindBankTransactionByBalance :one
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND cash_box_id = @cash_box_id
  AND transaction_date = @transaction_date AND debit_credit = @debit_credit
  AND amount = @amount AND balance_after = @balance_after
ORDER BY id
LIMIT 1;

-- 21 §5 kural 3: bakiye yoksa aynı hesap + tarih + yön + tutar + normalize
-- açıklamanın ilk 30 karakteri → olası mükerrer.
-- name: FindBankTransactionLookalike :one
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND cash_box_id = @cash_box_id
  AND transaction_date = @transaction_date AND debit_credit = @debit_credit
  AND amount = @amount AND left(description_normalized, 30) = @description_prefix::text
ORDER BY id
LIMIT 1;

-- name: GetBankTransaction :one
SELECT * FROM bank_transactions WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: InsertBankTransaction :one
INSERT INTO bank_transactions (
    taxpayer_id, cash_box_id, source_system, source_id, capture_method, booked_at, has_time,
    bank_sequence, transaction_date, value_date, debit_credit, amount, currency, balance_after,
    balance_before, description, description_extra, description_normalized, bank_reference,
    receipt_number, bank_txn_code, mt940_code, channel, counterparty_name, counterparty_iban,
    counterparty_tax_number, counterparty_identity_number, counterparty_bank_code, eft_query_number,
    payment_purpose, is_cancelled, possible_duplicate_of, period, raw_payload, content_hash
) VALUES (
    @taxpayer_id, @cash_box_id, @source_system, @source_id, @capture_method, @booked_at, @has_time,
    sqlc.narg(bank_sequence), @transaction_date, sqlc.narg(value_date), @debit_credit, @amount, @currency, sqlc.narg(balance_after),
    sqlc.narg(balance_before), @description, sqlc.narg(description_extra), @description_normalized, sqlc.narg(bank_reference),
    sqlc.narg(receipt_number), sqlc.narg(bank_txn_code), sqlc.narg(mt940_code), sqlc.narg(channel), sqlc.narg(counterparty_name), sqlc.narg(counterparty_iban),
    sqlc.narg(counterparty_tax_number), sqlc.narg(counterparty_identity_number), sqlc.narg(counterparty_bank_code), sqlc.narg(eft_query_number),
    sqlc.narg(payment_purpose), @is_cancelled, sqlc.narg(possible_duplicate_of), @period, @raw_payload, sqlc.narg(content_hash)
)
RETURNING *;

-- Daha öncelikli bir kanal geldiğinde birincil kaynağı değiştirir.
-- name: SetBankTransactionPrimarySource :exec
UPDATE bank_transactions
SET source_system = @source_system, source_id = @source_id,
    capture_method = @capture_method, raw_payload = @raw_payload
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- Düşük öncelikli kanaldaki ek bilgi yalnızca boş alanlara yazılır (21 §5).
-- name: FillBankTransactionGaps :exec
UPDATE bank_transactions SET
    description_extra = COALESCE(description_extra, sqlc.narg(description_extra)),
    bank_reference = COALESCE(bank_reference, sqlc.narg(bank_reference)),
    receipt_number = COALESCE(receipt_number, sqlc.narg(receipt_number)),
    bank_txn_code = COALESCE(bank_txn_code, sqlc.narg(bank_txn_code)),
    mt940_code = COALESCE(mt940_code, sqlc.narg(mt940_code)),
    counterparty_name = COALESCE(counterparty_name, sqlc.narg(counterparty_name)),
    counterparty_iban = COALESCE(counterparty_iban, sqlc.narg(counterparty_iban)),
    counterparty_tax_number = COALESCE(counterparty_tax_number, sqlc.narg(counterparty_tax_number)),
    counterparty_identity_number = COALESCE(counterparty_identity_number, sqlc.narg(counterparty_identity_number)),
    payment_purpose = COALESCE(payment_purpose, sqlc.narg(payment_purpose)),
    balance_after = COALESCE(balance_after, sqlc.narg(balance_after))
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: InsertBankTransactionSource :exec
INSERT INTO bank_transaction_sources (
    taxpayer_id, bank_transaction_id, cash_box_id, source_system, capture_method, source_id, import_id, raw_payload
) VALUES (
    @taxpayer_id, @bank_transaction_id, @cash_box_id, @source_system, @capture_method, @source_id, sqlc.narg(import_id), @raw_payload
);

-- name: ListBankTransactionsToClassify :many
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period)::text)
  AND (NOT @only_unclassified::boolean OR classified_at IS NULL)
  AND kind_source IS DISTINCT FROM 'manual'
ORDER BY transaction_date, bank_sequence NULLS LAST, id;

-- name: SetBankTransactionClassification :exec
UPDATE bank_transactions
SET kind = @kind, kind_detail = @kind_detail, kind_source = @kind_source,
    kind_confidence = @kind_confidence, kind_reason = sqlc.narg(kind_reason), classified_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- Güveni eşiğin altındaki sınıflandırmalar: müşavire "kontrol et" (21 §4).
-- name: ListBankTransactionsNeedingReview :many
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND period = @period AND NOT is_cancelled
  AND kind_source IS DISTINCT FROM 'manual'
  AND (kind_confidence IS NULL OR kind_confidence < @threshold::numeric)
ORDER BY kind_confidence NULLS FIRST, transaction_date, id;

-- name: ListBankRules :many
SELECT * FROM bank_rules WHERE taxpayer_id = @taxpayer_id ORDER BY id;

-- name: UpsertBankRule :one
INSERT INTO bank_rules (taxpayer_id, match_type, match_value, debit_credit, kind, kind_detail, created_by)
VALUES (@taxpayer_id, @match_type, @match_value, sqlc.narg(debit_credit), @kind, @kind_detail, sqlc.narg(created_by))
ON CONFLICT (taxpayer_id, match_type, match_value, debit_credit)
DO UPDATE SET kind = EXCLUDED.kind, kind_detail = EXCLUDED.kind_detail, created_by = EXCLUDED.created_by
RETURNING *;

-- name: IncrementBankRuleHits :exec
UPDATE bank_rules SET hits = hits + 1 WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- Sınıflandırma adım 5 için mükellefin carileri (VKN/TCKN, IBAN, tür).
-- name: ListCustomersForBankClassification :many
SELECT id, type, tax_number, identity_number, bank_accounts FROM customers
WHERE taxpayer_id = @taxpayer_id AND deleted_at_source IS NULL;
