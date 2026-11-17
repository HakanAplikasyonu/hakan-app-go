-- Cari mutabakat, BA/BS ve mutabakat mektupları (#389).

-- name: ReconciliationCounterpartyInvoices :many
-- Karşı firma bazında: dönem sonuna kadarki faturalardan işaretli bakiye
-- (satış +, alış −; TL) ve dönemin BA (alış) / BS (satış) KDV hariç
-- tutarı ile belge sayısı.
SELECT x.tax_id::text AS tax_id,
       MAX(x.name)::text AS name,
       COALESCE(SUM(x.signed_payable), 0)::numeric AS invoice_balance,
       COALESCE(SUM(x.net) FILTER (WHERE x.direction = 'incoming' AND x.period = @period::text), 0)::numeric AS ba_amount,
       COUNT(*) FILTER (WHERE x.direction = 'incoming' AND x.period = @period::text)::int AS ba_count,
       COALESCE(SUM(x.net) FILTER (WHERE x.direction = 'outgoing' AND x.period = @period::text), 0)::numeric AS bs_amount,
       COUNT(*) FILTER (WHERE x.direction = 'outgoing' AND x.period = @period::text)::int AS bs_count
FROM (
    SELECT COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, '')) AS tax_id,
           COALESCE(i.customer_name, '') AS name,
           i.direction, i.period,
           (CASE WHEN i.direction = 'outgoing' THEN 1 ELSE -1 END) *
             (CASE WHEN i.currency = 'TRY' THEN i.payable_amount
                   ELSE COALESCE(i.payable_amount_try, i.payable_amount * COALESCE(i.exchange_rate, 0)) END) AS signed_payable,
           CASE WHEN i.currency = 'TRY' THEN i.tax_exclusive_amount
                ELSE i.tax_exclusive_amount * COALESCE(i.exchange_rate, 0) END AS net
    FROM invoices i
    WHERE i.taxpayer_id = @taxpayer_id AND i.status_normalized = 'active' AND i.issue_date <= @date_to::date
) x
WHERE x.tax_id IS NOT NULL AND x.tax_id ~ '^[0-9]{10,11}$'
GROUP BY x.tax_id
ORDER BY x.tax_id;

-- name: ReconciliationCounterpartyPayments :many
-- Faturalara eşleşmiş banka hareketleri, karşı firma bazında işaretli
-- (gelen para alacağı azaltır −, giden para borcu azaltır +).
SELECT COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, ''))::text AS tax_id,
       COALESCE(SUM(CASE WHEN t.debit_credit = 'A' THEN -m.amount ELSE m.amount END), 0)::numeric AS paid
FROM bank_transaction_matches m
JOIN bank_transactions t ON t.taxpayer_id = m.taxpayer_id AND t.id = m.bank_transaction_id
JOIN invoices i ON i.taxpayer_id = m.taxpayer_id AND i.id = m.target_id
WHERE m.taxpayer_id = @taxpayer_id AND t.taxpayer_id = @taxpayer_id AND i.taxpayer_id = @taxpayer_id
  AND m.target_type = 'invoice' AND t.transaction_date <= @date_to::date AND NOT t.is_cancelled
  AND t.currency = 'TRY'
  AND COALESCE(NULLIF(i.customer_tax_number, ''), NULLIF(i.customer_identity_number, '')) IS NOT NULL
GROUP BY 1;

-- name: ReconciliationCounterpartyAccounts :many
-- Cari hafızası (#90): karşı firmanın 120 / 320 (ve benzeri) alt hesabı.
SELECT counterparty_tax_id::text AS counterparty_tax_id, main_code::text AS main_code, account_code, account_name
FROM counterparty_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
ORDER BY counterparty_tax_id, main_code;

-- name: ReconciliationCounterpartyContacts :many
-- Karşı firmanın cari kartındaki e-posta (mektup alıcısı önerisi).
SELECT COALESCE(NULLIF(c.tax_number, ''), NULLIF(c.identity_number, ''))::text AS tax_id,
       MAX(NULLIF(c.email, ''))::text AS email, MAX(c.name)::text AS name
FROM customers c
WHERE c.taxpayer_id = @taxpayer_id
  AND COALESCE(NULLIF(c.tax_number, ''), NULLIF(c.identity_number, '')) IS NOT NULL
GROUP BY 1;

-- name: InsertCounterpartyReconciliations :execrows
INSERT INTO counterparty_reconciliations (organization_id, engagement_id, run_id, counterparty_tax_id, counterparty_name,
    account_code, ledger_balance, source_balance, difference, result, ba_amount, ba_count, bs_amount, bs_count,
    declared_ba_amount, declared_ba_count, declared_bs_amount, declared_bs_count, ba_result, bs_result, status, note)
SELECT @organization_id, @engagement_id, @run_id, a.counterparty_tax_id, a.counterparty_name,
       a.account_code, a.ledger_balance, a.source_balance, a.difference, a.result, a.ba_amount, a.ba_count, a.bs_amount, a.bs_count,
       a.declared_ba_amount, a.declared_ba_count, a.declared_bs_amount, a.declared_bs_count, a.ba_result, a.bs_result, a.status, a.note
FROM jsonb_to_recordset(@rows::jsonb) AS a(counterparty_tax_id text, counterparty_name text, account_code text,
    ledger_balance numeric, source_balance numeric, difference numeric, result text,
    ba_amount numeric, ba_count int, bs_amount numeric, bs_count int,
    declared_ba_amount numeric, declared_ba_count int, declared_bs_amount numeric, declared_bs_count int,
    ba_result text, bs_result text, status text, note text);

-- name: ListCounterpartyReconciliations :many
SELECT c.*, s.uuid AS review_uuid, s.status AS review_status
FROM counterparty_reconciliations c
LEFT JOIN suggestions s ON s.organization_id = c.organization_id AND s.id = c.review_suggestion_id
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id AND c.run_id = @run_id
ORDER BY c.counterparty_name, c.counterparty_tax_id;

-- name: GetCounterpartyReconciliation :one
SELECT c.*, s.uuid AS review_uuid, s.status AS review_status, r.period AS run_period, r.uuid AS run_uuid
FROM counterparty_reconciliations c
JOIN reconciliation_runs r ON r.engagement_id = c.engagement_id AND r.id = c.run_id
LEFT JOIN suggestions s ON s.organization_id = c.organization_id AND s.id = c.review_suggestion_id
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id AND c.uuid = @uuid;

-- name: GetCounterpartyReconciliationByID :one
SELECT * FROM counterparty_reconciliations
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
FOR UPDATE;

-- name: UpdateCounterpartyReconciliationDecision :one
UPDATE counterparty_reconciliations
SET status = @status, note = sqlc.narg(note), decided_by = sqlc.narg(decided_by), decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: SetCounterpartyReconciliationReview :exec
UPDATE counterparty_reconciliations
SET status = 'in_review', review_suggestion_id = @review_suggestion_id, decided_by = sqlc.narg(decided_by), decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: ResolveLatestCounterpartyReconciliation :exec
-- Karşı tarafın "mutabıkız" yanıtı: en son çalıştırmadaki açık satır çözülür.
UPDATE counterparty_reconciliations c
SET status = 'resolved', note = COALESCE(c.note, @note::text), decided_at = NOW()
WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
  AND c.counterparty_tax_id = @counterparty_tax_id AND c.status = 'open'
  AND c.run_id = (SELECT r.id FROM reconciliation_runs r
                  WHERE r.organization_id = @organization_id AND r.engagement_id = @engagement_id
                  ORDER BY r.period DESC, r.id DESC LIMIT 1);

-- name: ListBabsDeclarations :many
SELECT * FROM babs_declarations
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period::text
  AND (sqlc.narg(form_type)::text IS NULL OR form_type = sqlc.narg(form_type)::text)
ORDER BY form_type, counterparty_name, counterparty_tax_id;

-- name: UpsertBabsDeclaration :one
INSERT INTO babs_declarations (organization_id, engagement_id, taxpayer_id, period, form_type, counterparty_tax_id,
    counterparty_name, document_count, amount, source, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, @form_type, @counterparty_tax_id,
    @counterparty_name, @document_count, @amount, @source, sqlc.narg(created_by))
ON CONFLICT (engagement_id, period, form_type, counterparty_tax_id) DO UPDATE
SET counterparty_name = EXCLUDED.counterparty_name, document_count = EXCLUDED.document_count,
    amount = EXCLUDED.amount, source = EXCLUDED.source
RETURNING *;

-- name: DeleteBabsDeclaration :execrows
DELETE FROM babs_declarations
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: InsertReconciliationLetter :one
INSERT INTO reconciliation_letters (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, counterparty_name,
    recipient_email, account_code, as_of_date, balance, snapshot, status, token_digest, token_expires_at, next_attempt_at, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @counterparty_tax_id, @counterparty_name,
    sqlc.narg(recipient_email), sqlc.narg(account_code), @as_of_date, @balance, @snapshot, @status,
    sqlc.narg(token_digest), sqlc.narg(token_expires_at), sqlc.narg(next_attempt_at), sqlc.narg(created_by))
RETURNING *;

-- name: SetReconciliationLetterToken :exec
UPDATE reconciliation_letters SET token_digest = @token_digest, token_expires_at = @token_expires_at
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: ListReconciliationLetters :many
SELECT * FROM reconciliation_letters
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(counterparty_tax_id)::text IS NULL OR counterparty_tax_id = sqlc.narg(counterparty_tax_id)::text)
ORDER BY id DESC
LIMIT @lim;

-- name: GetReconciliationLetter :one
SELECT * FROM reconciliation_letters
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: QueueReconciliationLetter :one
UPDATE reconciliation_letters
SET status = 'queued', recipient_email = @recipient_email, attempts = 0, last_error = NULL, next_attempt_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: RespondReconciliationLetter :one
UPDATE reconciliation_letters
SET status = 'responded', response = @response, response_balance = sqlc.narg(response_balance),
    response_note = sqlc.narg(response_note), responder_name = sqlc.narg(responder_name),
    response_channel = @response_channel, responded_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- tenant-lint: cross-org yanıt bağlantısı jetonla açılır (rls.System içinde, jetonun özeti tekil).
-- name: GetReconciliationLetterByToken :one
SELECT l.*, e.uuid AS engagement_uuid
FROM reconciliation_letters l
JOIN engagements e ON e.organization_id = l.organization_id AND e.id = l.engagement_id
WHERE l.token_digest = @token_digest;

-- tenant-lint: cross-org worker taraması (rls.System): sırası gelen mektuplar.
-- name: ClaimReconciliationLetters :many
UPDATE reconciliation_letters l
SET attempts = l.attempts + 1, next_attempt_at = NOW() + INTERVAL '10 minutes'
WHERE l.id IN (
    SELECT q.id FROM reconciliation_letters q
    WHERE q.status = 'queued' AND q.next_attempt_at <= NOW()
    ORDER BY q.next_attempt_at
    LIMIT @lim
    FOR UPDATE SKIP LOCKED)
RETURNING l.*;

-- tenant-lint: cross-org worker taraması (rls.System).
-- name: MarkReconciliationLetterSent :exec
UPDATE reconciliation_letters SET status = 'sent', sent_at = NOW(), last_error = NULL, next_attempt_at = NULL
WHERE id = @id AND status = 'queued';

-- tenant-lint: cross-org worker taraması (rls.System).
-- name: MarkReconciliationLetterRetry :exec
UPDATE reconciliation_letters SET last_error = @last_error, next_attempt_at = @next_attempt_at
WHERE id = @id AND status = 'queued';

-- tenant-lint: cross-org worker taraması (rls.System).
-- name: MarkReconciliationLetterFailed :exec
UPDATE reconciliation_letters SET status = 'failed', last_error = @last_error, next_attempt_at = NULL
WHERE id = @id AND status = 'queued';

-- name: PreviousReconciliationRun :one
-- Aynı dönemin bir önceki çalıştırması (cari kararlarını taşımak için).
SELECT * FROM reconciliation_runs
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period::text AND id < @before_id
ORDER BY id DESC
LIMIT 1;

-- name: UpdateReconciliationRunSummary :exec
UPDATE reconciliation_runs SET summary = @summary
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;
