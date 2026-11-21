-- Banka hareketi → mizan hesabı (issue #383).

-- name: ListBankTransactionsForPosting :many
-- Dönemin iptal edilmemiş hareketleri, varsa önerileri ve eşleşen faturanın
-- karşı taraf vergi numarası ile.
SELECT t.id, t.uuid, t.cash_box_id, t.transaction_date, t.debit_credit, t.amount, t.currency,
       t.description, t.counterparty_name, t.counterparty_iban, t.counterparty_tax_number,
       t.counterparty_identity_number, t.kind, t.kind_detail, t.kind_confidence, t.period,
       p.id AS posting_id, p.status AS posting_status, p.account_code AS posting_account_code,
       p.suggestion_id AS posting_suggestion_id, p.confidence AS posting_confidence,
       COALESCE((SELECT COALESCE(i.customer_tax_number, i.customer_identity_number)
        FROM bank_transaction_matches m
        JOIN invoices i ON i.taxpayer_id = m.taxpayer_id AND i.id = m.target_id
        WHERE m.taxpayer_id = t.taxpayer_id AND m.bank_transaction_id = t.id AND m.target_type = 'invoice'
        ORDER BY m.amount DESC LIMIT 1), '')::text AS matched_tax_id,
       COALESCE((SELECT i.customer_name
        FROM bank_transaction_matches m
        JOIN invoices i ON i.taxpayer_id = m.taxpayer_id AND i.id = m.target_id
        WHERE m.taxpayer_id = t.taxpayer_id AND m.bank_transaction_id = t.id AND m.target_type = 'invoice'
        ORDER BY m.amount DESC LIMIT 1), '')::text AS matched_name
FROM bank_transactions t
LEFT JOIN bank_transaction_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.bank_transaction_id = t.id
WHERE t.taxpayer_id = @taxpayer_id AND t.period = @period AND NOT t.is_cancelled
ORDER BY t.transaction_date, t.id;

-- name: GetBankTransactionForPosting :one
SELECT t.id, t.uuid, t.cash_box_id, t.transaction_date, t.debit_credit, t.amount, t.currency,
       t.description, t.counterparty_name, t.counterparty_iban, t.counterparty_tax_number,
       t.counterparty_identity_number, t.kind, t.kind_detail, t.period
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id AND (t.id = sqlc.narg(id)::bigint OR t.uuid = sqlc.narg(uuid)::uuid);

-- name: GetBankPosting :one
SELECT * FROM bank_transaction_postings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND bank_transaction_id = @bank_transaction_id;

-- name: UpsertBankPosting :one
-- Öneri yazılır; onaylanmış bir hareketin önerisi değişmez.
INSERT INTO bank_transaction_postings (organization_id, engagement_id, taxpayer_id, bank_transaction_id,
                                       account_code, account_name, status, confidence, source, reason, suggestion_id)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @bank_transaction_id, @account_code, @account_name,
        'suggested', @confidence, @source, @reason, sqlc.narg(suggestion_id))
ON CONFLICT (engagement_id, bank_transaction_id) DO UPDATE
SET account_code = EXCLUDED.account_code, account_name = EXCLUDED.account_name, confidence = EXCLUDED.confidence,
    source = EXCLUDED.source, reason = EXCLUDED.reason, suggestion_id = EXCLUDED.suggestion_id
WHERE bank_transaction_postings.status = 'suggested'
RETURNING *;

-- name: ApproveBankPosting :one
UPDATE bank_transaction_postings
SET status = 'approved', account_code = @account_code, account_name = @account_name,
    source = @source, suggestion_id = sqlc.narg(suggestion_id), approved_by = sqlc.narg(approved_by), approved_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND bank_transaction_id = @bank_transaction_id
RETURNING *;

-- name: UnapproveBankPosting :execrows
-- Geri alınan karar: öneri incelemeye döner; fişe girmiş hareket fişte kalır.
UPDATE bank_transaction_postings
SET status = 'suggested', account_code = @account_code, approved_by = NULL, approved_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND bank_transaction_id = @bank_transaction_id;

-- name: CountBankPostings :one
SELECT count(*) FILTER (WHERE p.id IS NULL)::bigint AS unassigned,
       count(*) FILTER (WHERE p.status = 'suggested')::bigint AS suggested,
       count(*) FILTER (WHERE p.status = 'approved' AND p.journal_draft_uuid IS NULL)::bigint AS approved,
       count(*) FILTER (WHERE p.journal_draft_uuid IS NOT NULL)::bigint AS drafted,
       count(*)::bigint AS total
FROM bank_transactions t
LEFT JOIN bank_transaction_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.bank_transaction_id = t.id
WHERE t.taxpayer_id = @taxpayer_id AND NOT t.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period)::text);

-- name: ListBankPostings :many
-- Ekran listesi: hareket + öneri + inceleme önerisi; durum süzgeci
-- unassigned (önerisi yok) / suggested / approved (fişe girmedi) / drafted.
SELECT t.id, t.uuid, t.transaction_date, t.debit_credit, t.amount, t.currency, t.description,
       t.counterparty_name, t.counterparty_iban, t.kind, t.kind_detail, t.match_status,
       cb.uuid AS cash_box_uuid, cb.name AS cash_box_name, cb.iban AS cash_box_iban,
       p.account_code, p.account_name, p.status, p.confidence, p.source, p.reason,
       p.journal_draft_uuid, p.approved_at, s.uuid AS suggestion_uuid, s.status AS suggestion_status
FROM bank_transactions t
JOIN cash_boxes cb ON cb.taxpayer_id = t.taxpayer_id AND cb.id = t.cash_box_id
LEFT JOIN bank_transaction_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.bank_transaction_id = t.id
LEFT JOIN suggestions s
       ON s.organization_id = @organization_id AND s.engagement_id = @engagement_id AND s.id = p.suggestion_id
WHERE t.taxpayer_id = @taxpayer_id AND NOT t.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'unassigned' AND p.id IS NULL)
       OR (sqlc.narg(status)::text = 'suggested' AND p.status = 'suggested')
       OR (sqlc.narg(status)::text = 'approved' AND p.status = 'approved' AND p.journal_draft_uuid IS NULL)
       OR (sqlc.narg(status)::text = 'drafted' AND p.journal_draft_uuid IS NOT NULL))
ORDER BY t.transaction_date DESC, t.id DESC
LIMIT @lim OFFSET @off;

-- name: ListApprovedBankPostingsWithoutDraft :many
SELECT p.bank_transaction_id, p.account_code, p.account_name, p.confidence,
       p.commission_amount, p.commission_account_code,
       t.uuid, t.cash_box_id, t.transaction_date, t.debit_credit, t.amount, t.currency, t.description,
       t.counterparty_name, cb.name AS cash_box_name,
       COALESCE(l.account_code, '102')::text AS bank_account_code,
       -- Ofis içi virman (#710): karşı hareket; fiş yalnız çıkan tarafta.
       COALESCE((SELECT m.target_id FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = t.taxpayer_id AND m.bank_transaction_id = t.id AND m.target_type = 'bank_transaction'
                 LIMIT 1), 0)::bigint AS transfer_partner_id
FROM bank_transaction_postings p
JOIN bank_transactions t ON t.taxpayer_id = p.taxpayer_id AND t.id = p.bank_transaction_id
JOIN cash_boxes cb ON cb.taxpayer_id = t.taxpayer_id AND cb.id = t.cash_box_id
LEFT JOIN bank_ledger_accounts l
       ON l.organization_id = p.organization_id AND l.engagement_id = p.engagement_id AND l.cash_box_id = t.cash_box_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.taxpayer_id = @taxpayer_id
  AND p.status = 'approved' AND p.journal_draft_uuid IS NULL AND NOT t.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period)::text)
ORDER BY t.cash_box_id, t.transaction_date, t.id;

-- name: SetBankPostingsDraft :exec
UPDATE bank_transaction_postings SET journal_draft_uuid = @journal_draft_uuid
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND bank_transaction_id = ANY(@bank_transaction_ids::bigint[]);

-- name: ListBankLedgerAccounts :many
SELECT cb.uuid AS cash_box_uuid, cb.name, cb.iban, cb.bank_id, cb.currency, l.account_code
FROM cash_boxes cb
LEFT JOIN bank_ledger_accounts l
       ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.cash_box_id = cb.id
WHERE cb.taxpayer_id = @taxpayer_id
ORDER BY cb.id;

-- name: UpsertBankLedgerAccount :exec
INSERT INTO bank_ledger_accounts (organization_id, engagement_id, taxpayer_id, cash_box_id, account_code, updated_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @cash_box_id, @account_code, sqlc.narg(updated_by))
ON CONFLICT (engagement_id, cash_box_id) DO UPDATE
SET account_code = EXCLUDED.account_code, updated_by = EXCLUDED.updated_by;

-- name: SetBankPostingSuggestion :exec
UPDATE bank_transaction_postings SET suggestion_id = @suggestion_id
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND bank_transaction_id = @bank_transaction_id
  AND status = 'suggested';

-- name: GetBankPostingView :one
SELECT p.*, s.uuid AS suggestion_uuid, s.status AS suggestion_status
FROM bank_transaction_postings p
LEFT JOIN suggestions s ON s.organization_id = p.organization_id AND s.engagement_id = p.engagement_id AND s.id = p.suggestion_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id AND p.bank_transaction_id = @bank_transaction_id;

-- name: BankTransactionBookedAccounts :many
-- Banka listesindeki "hesaba işlendi" göstergesi (#565): onaylanmış karşı
-- hesabı olan hareketler.
SELECT t.uuid AS bank_transaction_uuid, p.account_code, p.account_name
FROM bank_transaction_postings p
JOIN bank_transactions t ON t.taxpayer_id = p.taxpayer_id AND t.id = p.bank_transaction_id
WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id
  AND t.taxpayer_id = @taxpayer_id AND t.uuid = ANY(@uuids::uuid[]) AND p.status = 'approved';

-- POS komisyonu (#565): yalnız fişe girmemiş harekette; boş değer siler.
-- name: SetBankPostingCommission :execrows
UPDATE bank_transaction_postings
SET commission_amount = sqlc.narg(commission_amount), commission_account_code = sqlc.narg(commission_account_code)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND taxpayer_id = @taxpayer_id
  AND bank_transaction_id = @bank_transaction_id AND journal_draft_uuid IS NULL;
