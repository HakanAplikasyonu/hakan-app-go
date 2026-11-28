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
-- açıklamanın ilk 30 karakteri → olası mükerrer. Aynı yüklemede az önce
-- yazılan hareketler (exclude_ids) sayılmaz: bir dosyada aynı gün aynı
-- tutarlı iki ödeme iki ayrı harekettir, birbirinin mükerreri değildir.
-- name: FindBankTransactionLookalike :one
SELECT * FROM bank_transactions
WHERE taxpayer_id = @taxpayer_id AND cash_box_id = @cash_box_id
  AND transaction_date = @transaction_date AND debit_credit = @debit_credit
  AND amount = @amount AND left(description_normalized, 30) = @description_prefix::text
  AND NOT (id = ANY(COALESCE(@exclude_ids::bigint[], '{}'::bigint[])))
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

-- name: FindCashBoxByAccountNumber :many
SELECT * FROM cash_boxes
WHERE taxpayer_id = @taxpayer_id AND bank_account_number = @bank_account_number;

-- name: UpsertBankDailySummary :exec
INSERT INTO bank_daily_summaries (
    taxpayer_id, cash_box_id, date, opening_balance, closing_balance, total_credit, total_debit, transaction_count, source
) VALUES (
    @taxpayer_id, @cash_box_id, @date, sqlc.narg(opening_balance), sqlc.narg(closing_balance),
    sqlc.narg(total_credit), sqlc.narg(total_debit), sqlc.narg(transaction_count), @source
)
ON CONFLICT (taxpayer_id, cash_box_id, date, source) DO UPDATE SET
    opening_balance = EXCLUDED.opening_balance, closing_balance = EXCLUDED.closing_balance,
    total_credit = EXCLUDED.total_credit, total_debit = EXCLUDED.total_debit,
    transaction_count = EXCLUDED.transaction_count;

-- Ardışık ekstrelerde önceki kapanış = bu açılış (23 §3.4).
-- name: FindPreviousDailySummary :one
SELECT * FROM bank_daily_summaries
WHERE taxpayer_id = @taxpayer_id AND cash_box_id = @cash_box_id AND date < @date
  AND closing_balance IS NOT NULL
ORDER BY date DESC, id DESC
LIMIT 1;

-- name: InsertBankBalanceSnapshot :exec
INSERT INTO bank_balance_snapshots (
    taxpayer_id, cash_box_id, captured_at, balance, available_balance, source, import_id
) VALUES (
    @taxpayer_id, @cash_box_id, @captured_at, @balance, sqlc.narg(available_balance), @source, sqlc.narg(import_id)
);

-- Yükleme geri alma (21 §2): yalnızca o yüklemenin kaynak satırları silinir.
-- name: DeleteBankTransactionSourcesByImport :many
DELETE FROM bank_transaction_sources
WHERE taxpayer_id = @taxpayer_id AND import_id = @import_id
RETURNING bank_transaction_id;

-- name: ListBankTransactionSources :many
SELECT * FROM bank_transaction_sources
WHERE taxpayer_id = @taxpayer_id AND bank_transaction_id = @bank_transaction_id
ORDER BY id;

-- name: DeleteBankTransaction :exec
DELETE FROM bank_transactions WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: ClearPossibleDuplicateLinks :exec
UPDATE bank_transactions SET possible_duplicate_of = NULL
WHERE taxpayer_id = @taxpayer_id AND possible_duplicate_of = @id;

-- name: DeleteBankBalanceSnapshotsByImport :exec
DELETE FROM bank_balance_snapshots WHERE taxpayer_id = @taxpayer_id AND import_id = @import_id;

-- name: GetCashBoxByUUID :one
SELECT * FROM cash_boxes WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- Müşavirin hesap düzenlemesi (#780): ad, banka, şube, IBAN, para birimi,
-- kullanımda / pasif. Alanlar Go tarafında mevcut satırla birleştirilir.
-- name: UpdateCashBox :one
UPDATE cash_boxes SET
    name = @name, bank_id = sqlc.narg(bank_id), iban = sqlc.narg(iban), currency = @currency,
    bank_account_number = sqlc.narg(bank_account_number), bank_branch_code = sqlc.narg(bank_branch_code),
    deactivated_at = sqlc.narg(deactivated_at), updated_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid
RETURNING *;

-- Dosya hesap numarası taşımıyorsa, bankası belli ve mükellefin o bankada
-- kullanımdaki tek hesabı varsa ekstre ona yüklenir (#780).
-- name: ListActiveCashBoxesByBank :many
SELECT * FROM cash_boxes
WHERE taxpayer_id = @taxpayer_id AND bank_id = @bank_id AND type = 'bank_account' AND deactivated_at IS NULL
ORDER BY id;

-- name: ListBankTransactionsPage :many
-- Banka hareketleri sekmesi (#329): mükellefin hareketleri, yeniden eskiye.
-- İptal edilen hareket listelenmez.
SELECT bt.uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.description, bt.counterparty_name, bt.kind, bt.kind_detail,
       bt.match_status, bt.period, (bt.possible_duplicate_of IS NOT NULL)::boolean AS possible_duplicate,
       cb.uuid AS cash_box_uuid, cb.name AS cash_box_name
FROM bank_transactions bt
JOIN cash_boxes cb ON cb.id = bt.cash_box_id AND cb.taxpayer_id = bt.taxpayer_id
WHERE bt.taxpayer_id = @taxpayer_id AND NOT bt.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR bt.period = sqlc.narg(period)::text)
  AND (sqlc.narg(match_status)::text IS NULL OR bt.match_status = sqlc.narg(match_status)::text)
  AND (sqlc.narg(cash_box_uuid)::uuid IS NULL OR cb.uuid = sqlc.narg(cash_box_uuid)::uuid)
ORDER BY bt.transaction_date DESC, bt.id DESC
LIMIT @lim OFFSET @off;

-- name: SummarizeBankTransactions :one
-- Aynı süzgeçle sayı, giriş (A, alacak) / çıkış (B, borç) toplamı ve eşleşme
-- durumları.
SELECT count(*)::bigint AS total,
       COALESCE(sum(bt.amount) FILTER (WHERE bt.debit_credit = 'A'), 0)::numeric(18,4) AS in_total,
       COALESCE(sum(bt.amount) FILTER (WHERE bt.debit_credit = 'B'), 0)::numeric(18,4) AS out_total,
       count(*) FILTER (WHERE bt.match_status = 'unmatched')::bigint AS unmatched,
       count(*) FILTER (WHERE bt.match_status = 'suggested')::bigint AS suggested,
       count(*) FILTER (WHERE bt.match_status = 'matched')::bigint AS matched,
       -- Faturayla eşleşmemiş ama karşı hesabı onaylanmış (vergi, SGK, maaş,
       -- masraf …): iş bitmiştir, bekleyen sayılmaz (#710).
       count(*) FILTER (WHERE bt.match_status = 'unmatched' AND EXISTS (
           SELECT 1 FROM bank_transaction_postings p
           WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id
             AND p.taxpayer_id = bt.taxpayer_id AND p.bank_transaction_id = bt.id AND p.status = 'approved'))::bigint AS unmatched_booked
FROM bank_transactions bt
JOIN cash_boxes cb ON cb.id = bt.cash_box_id AND cb.taxpayer_id = bt.taxpayer_id
WHERE bt.taxpayer_id = @taxpayer_id AND NOT bt.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR bt.period = sqlc.narg(period)::text)
  AND (sqlc.narg(cash_box_uuid)::uuid IS NULL OR cb.uuid = sqlc.narg(cash_box_uuid)::uuid);

-- name: ExportBankTransactions :many
-- Banka hareketleri Excel dışa aktarımı (#395): listeyle aynı süzgeç, ekstre
-- kolonları ve hesabın IBAN'ı; eskiden yeniye.
SELECT bt.transaction_date, bt.value_date, bt.debit_credit, bt.amount, bt.currency, bt.balance_after,
       bt.description, bt.bank_reference, bt.receipt_number, bt.channel,
       bt.counterparty_name, bt.counterparty_iban,
       COALESCE(bt.counterparty_tax_number, bt.counterparty_identity_number, '')::text AS counterparty_tax_id,
       bt.kind, bt.kind_detail, bt.match_status,
       cb.name AS cash_box_name, cb.iban AS cash_box_iban
FROM bank_transactions bt
JOIN cash_boxes cb ON cb.id = bt.cash_box_id AND cb.taxpayer_id = bt.taxpayer_id
WHERE bt.taxpayer_id = @taxpayer_id AND NOT bt.is_cancelled
  AND (sqlc.narg(period)::text IS NULL OR bt.period = sqlc.narg(period)::text)
  AND (sqlc.narg(match_status)::text IS NULL OR bt.match_status = sqlc.narg(match_status)::text)
  AND (sqlc.narg(cash_box_uuid)::uuid IS NULL OR cb.uuid = sqlc.narg(cash_box_uuid)::uuid)
ORDER BY cb.id, bt.transaction_date, bt.booked_at, bt.id
LIMIT @lim;

-- name: BankBalanceCheck :many
-- Dönemin ekstre bakiye kontrolü (#570): her hesapta ilk hareketin önceki
-- bakiyesi + giriş − çıkış, son hareketin sonraki bakiyesiyle karşılaştırılır.
-- Önceki bakiye yoksa sonraki bakiyeden geri hesaplanır.
WITH tx AS (
    SELECT bt.cash_box_id, bt.debit_credit, bt.amount, bt.transaction_date, bt.balance_after,
           COALESCE(bt.balance_before,
                    bt.balance_after - CASE WHEN bt.debit_credit = 'A' THEN bt.amount ELSE -bt.amount END) AS before_balance,
           row_number() OVER (PARTITION BY bt.cash_box_id
                              ORDER BY bt.transaction_date, bt.booked_at, bt.bank_sequence NULLS LAST, bt.id) AS rn_first,
           row_number() OVER (PARTITION BY bt.cash_box_id
                              ORDER BY bt.transaction_date DESC, bt.booked_at DESC, bt.bank_sequence DESC NULLS LAST, bt.id DESC) AS rn_last
    FROM bank_transactions bt
    WHERE bt.taxpayer_id = @taxpayer_id AND bt.period = @period::text AND NOT bt.is_cancelled
)
SELECT cb.uuid AS cash_box_uuid, cb.name AS cash_box_name, cb.iban, cb.currency,
       count(*)::bigint AS movements,
       count(*) FILTER (WHERE tx.balance_after IS NULL)::bigint AS without_balance,
       COALESCE(sum(tx.amount) FILTER (WHERE tx.debit_credit = 'A'), 0)::numeric(18,4) AS in_total,
       COALESCE(sum(tx.amount) FILTER (WHERE tx.debit_credit = 'B'), 0)::numeric(18,4) AS out_total,
       max(tx.before_balance) FILTER (WHERE tx.rn_first = 1)::numeric(18,4) AS opening,
       max(tx.balance_after) FILTER (WHERE tx.rn_last = 1)::numeric(18,4) AS closing,
       min(tx.transaction_date)::date AS first_date,
       max(tx.transaction_date)::date AS last_date
FROM tx
JOIN cash_boxes cb ON cb.id = tx.cash_box_id AND cb.taxpayer_id = @taxpayer_id
GROUP BY cb.uuid, cb.name, cb.iban, cb.currency
ORDER BY cb.name, cb.uuid;

-- name: ListBankBalanceSuspects :many
-- Bakiye uyuşmazlığında bakılacak hareketler (#710): içe aktarmanın olası
-- mükerrer dediği ya da aynı hesapta aynı gün, yön ve tutarda ikizi olan
-- hareketler; ofisin hesaptan çıkardıkları da (geri alınabilsin diye).
SELECT bt.uuid, cb.uuid AS cash_box_uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.description, bt.is_cancelled, bt.excluded_at, (bt.possible_duplicate_of IS NOT NULL)::bool AS flagged
FROM bank_transactions bt
JOIN cash_boxes cb ON cb.id = bt.cash_box_id AND cb.taxpayer_id = bt.taxpayer_id
WHERE bt.taxpayer_id = @taxpayer_id AND bt.period = @period::text
  AND (bt.excluded_at IS NOT NULL
       OR (NOT bt.is_cancelled
           AND (bt.possible_duplicate_of IS NOT NULL
                OR EXISTS (SELECT 1 FROM bank_transactions o
                           WHERE o.taxpayer_id = bt.taxpayer_id AND o.cash_box_id = bt.cash_box_id AND o.id <> bt.id
                             AND NOT o.is_cancelled AND o.transaction_date = bt.transaction_date
                             AND o.debit_credit = bt.debit_credit AND o.amount = bt.amount))))
ORDER BY cb.uuid, bt.transaction_date, bt.amount, bt.id
LIMIT 100;

-- name: SetBankTransactionExcluded :one
-- Ofis hareketi hesaptan çıkarır ya da yeniden dahil eder (#710). Bankanın
-- iptal ettiği hareket (excluded_at boş, is_cancelled) dahil edilemez.
UPDATE bank_transactions
SET is_cancelled = @excluded::bool,
    excluded_at  = CASE WHEN @excluded::bool THEN now() ELSE NULL END,
    excluded_by  = CASE WHEN @excluded::bool THEN sqlc.narg(user_id)::bigint ELSE NULL END
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid
  AND (NOT is_cancelled OR excluded_at IS NOT NULL)
RETURNING id, period;
