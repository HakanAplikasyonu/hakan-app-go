-- Banka bakiyesi ile muhasebe (102) bakiyesi farkı ve nedenleri (#780 C).

-- name: BankDifferenceTransactions :many
-- Yılbaşından döneme TL hareketler (iptal edilenler de: fişe girmiş iptal
-- hareket mükerrer sayılır), karşı virman hareketiyle.
SELECT t.id, t.uuid, t.cash_box_id, t.transaction_date, t.debit_credit, t.amount,
       t.balance_before, t.balance_after, t.is_cancelled, t.period, t.description,
       (t.possible_duplicate_of IS NOT NULL)::bool AS flagged,
       COALESCE((SELECT m.target_id FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = t.taxpayer_id AND m.bank_transaction_id = t.id AND m.target_type = 'bank_transaction'
                 LIMIT 1), 0)::bigint AS transfer_partner_id
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id AND t.currency = 'TRY'
  AND t.period >= @from_period::text AND t.period <= @to_period::text
ORDER BY t.cash_box_id, t.transaction_date, t.booked_at, t.bank_sequence NULLS LAST, t.id;

-- name: BankDifferenceLedgerLines :many
-- Yılbaşından döneme 102 hesaplarına yazılmış fiş satırları (taslaklar dahil).
SELECT d.uuid AS draft_uuid, d.entry_no, d.period, d.entry_type, d.entry_date, d.status,
       l.line_no, l.account_code, l.description, l.debit, l.credit, l.source_kind, l.source_id
FROM journal_draft_lines l
JOIN journal_drafts d ON d.id = l.journal_draft_id AND d.engagement_id = l.engagement_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
  AND d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND d.period >= @from_period::text AND d.period <= @to_period::text
  AND l.account_code LIKE '102%'
ORDER BY d.entry_date, d.entry_no, l.line_no;

-- name: ListBankDifferenceResolutions :many
SELECT r.uuid, r.reason, r.action, r.amount, r.counter_account, r.explanation, r.journal_draft_uuid,
       r.created_at, cb.uuid AS cash_box_uuid,
       COALESCE(NULLIF(btrim(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, '')), ''), '')::text AS created_by_name
FROM bank_difference_resolutions r
JOIN cash_boxes cb ON cb.taxpayer_id = r.taxpayer_id AND cb.id = r.cash_box_id
LEFT JOIN users u ON u.id = r.created_by
WHERE r.organization_id = @organization_id AND r.engagement_id = @engagement_id AND r.period = @period
ORDER BY r.created_at, r.id;

-- name: InsertBankDifferenceResolution :one
INSERT INTO bank_difference_resolutions (organization_id, engagement_id, taxpayer_id, cash_box_id, period, reason,
                                         action, amount, counter_account, explanation, journal_draft_uuid, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @cash_box_id, @period, @reason, @action,
        sqlc.narg(amount), sqlc.narg(counter_account), @explanation, sqlc.narg(journal_draft_uuid), sqlc.narg(created_by))
RETURNING uuid, created_at;

-- name: DeleteBankDifferenceExplanation :execrows
-- Yalnız "açıklandı" işareti geri alınır; düzeltme fişi fişler ekranından silinir.
DELETE FROM bank_difference_resolutions
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND action = 'explained';

-- name: BankDifferenceBoxes :many
-- Mükellefin banka hesapları ve 102 hesapları (satırı yoksa "102").
SELECT cb.id, cb.uuid, cb.name, cb.iban, cb.currency,
       COALESCE(l.account_code, '102')::text AS account_code
FROM cash_boxes cb
LEFT JOIN bank_ledger_accounts l
       ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.cash_box_id = cb.id
WHERE cb.taxpayer_id = @taxpayer_id AND cb.type = 'bank_account'
ORDER BY cb.name, cb.id;
