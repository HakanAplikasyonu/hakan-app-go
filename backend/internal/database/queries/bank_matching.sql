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
-- booked_by_hand (#565): müşavir hareketi bu angajmanda kendi kararıyla bir
-- hesaba işledi: hesabı elle seçti (source manual) ya da öneriyi onayladı;
-- eşik üstü otomatik onay sayılmaz.
SELECT t.id, t.uuid, t.transaction_date, t.debit_credit, t.amount, t.currency, t.kind, t.kind_detail, t.match_status,
       t.counterparty_name, t.counterparty_iban, t.counterparty_tax_number, t.counterparty_identity_number,
       t.payment_purpose, t.description,
       EXISTS (SELECT 1 FROM bank_transaction_postings p
               LEFT JOIN suggestions s ON s.organization_id = p.organization_id AND s.engagement_id = p.engagement_id AND s.id = p.suggestion_id
               WHERE p.organization_id = @organization_id AND p.engagement_id = @engagement_id
                 AND p.bank_transaction_id = t.id AND p.status = 'approved'
                 AND (p.source = 'manual' OR s.status IN ('accepted', 'corrected')))::boolean AS booked_by_hand
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id AND t.period = @period AND NOT t.is_cancelled AND t.possible_duplicate_of IS NULL
ORDER BY t.transaction_date, t.id;

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

-- name: SearchInvoicesForBankMatch :many
-- Elle eşleştirme araması (#565): aday listesinin tarih ve tutar penceresi
-- dışında da; fatura no, unvan ya da VKN/TCKN ile hareketin yönündeki açık
-- faturalar. Süzgeçler isteğe bağlıdır: terim, düzenleme tarihi aralığı ve
-- fatura tutarı aralığı (hareketin para biriminde); boş olan uygulanmaz.
SELECT i.id, i.uuid, i.invoice_number, i.issue_date, i.customer_name,
       (CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END)::numeric AS comparable_amount,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = i.taxpayer_id AND m.target_type = 'invoice' AND m.target_id = i.id), 0)::numeric AS matched_amount
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.direction = @direction
  AND i.status_normalized = 'active'
  AND (i.currency = @currency::text OR (@currency::text = 'TRY' AND i.payable_amount_try IS NOT NULL))
  AND (sqlc.narg(term)::text IS NULL
       OR i.invoice_number ILIKE '%' || sqlc.narg(term)::text || '%'
       OR i.customer_name ILIKE '%' || sqlc.narg(term)::text || '%'
       OR i.customer_tax_number = sqlc.narg(term)::text
       OR i.customer_identity_number = sqlc.narg(term)::text)
  AND (sqlc.narg(date_from)::date IS NULL OR i.issue_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR i.issue_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(amount_min)::numeric IS NULL
       OR (CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END) >= sqlc.narg(amount_min)::numeric)
  AND (sqlc.narg(amount_max)::numeric IS NULL
       OR (CASE WHEN i.currency = @currency::text THEN i.payable_amount ELSE i.payable_amount_try END) <= sqlc.narg(amount_max)::numeric)
ORDER BY i.issue_date DESC, i.id
LIMIT 50;

-- Fiş satırı / cari hareketi eşleşmesi (#669 G). Satır angajman
-- kapsamlıdır: her sorgu satırı ve fişi organization_id + engagement_id ile,
-- eşleşmeyi taxpayer_id ile süzer.

-- name: ListBankTransactionMatchJournalLines :many
SELECT m.amount, m.method, m.matched_at,
       l.id AS line_id, l.line_no, l.account_code, l.description, l.document_no,
       j.uuid AS journal_draft_uuid, j.entry_no, j.entry_date, j.period, j.status AS journal_status
FROM bank_transaction_matches m
JOIN journal_draft_lines l
  ON l.organization_id = @organization_id AND l.engagement_id = @engagement_id AND l.id = m.target_id
JOIN journal_drafts j
  ON j.organization_id = @organization_id AND j.engagement_id = @engagement_id AND j.id = l.journal_draft_id
WHERE m.taxpayer_id = @taxpayer_id AND m.bank_transaction_id = @bank_transaction_id
  AND m.target_type = 'journal_line' AND j.taxpayer_id = @taxpayer_id
ORDER BY j.entry_date, j.entry_no, l.line_no;

-- Aday fiş satırları: hareketin yönündeki taraf (gelen para → alacak,
-- giden para → borç), banka hareketinden üretilmemiş satırlar. Arama terimi
-- boşsa tarih penceresindeki ve tutarı tutan satırlar; doluysa hesap kodu
-- (önek), satır / fiş açıklaması ya da belge no. cari_only: yalnız cari
-- ana hesapları (@cari_codes). Açık tutar, bu hareketin eşleşmeleri hariç.
-- name: SearchJournalLinesForBankMatch :many
SELECT l.id, l.line_no, l.account_code, l.description, l.document_no,
       (CASE WHEN @currency::text = 'TRY' THEN l.debit + l.credit ELSE COALESCE(l.foreign_amount, 0) END)::numeric AS line_amount,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = @taxpayer_id AND m.target_type = 'journal_line' AND m.target_id = l.id
                   AND m.bank_transaction_id <> @bank_transaction_id), 0)::numeric AS matched_amount,
       j.uuid AS journal_draft_uuid, j.entry_no, j.entry_date, j.period, j.status AS journal_status,
       j.description AS journal_description
FROM journal_draft_lines l
JOIN journal_drafts j
  ON j.organization_id = l.organization_id AND j.engagement_id = l.engagement_id AND j.id = l.journal_draft_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
  AND j.organization_id = @organization_id AND j.engagement_id = @engagement_id AND j.taxpayer_id = @taxpayer_id
  AND l.source_kind IS DISTINCT FROM 'bank_transaction'
  AND (CASE WHEN @side::text = 'credit' THEN l.credit > 0 ELSE l.debit > 0 END)
  AND (@currency::text = 'TRY' OR (l.currency = @currency::text AND l.foreign_amount IS NOT NULL))
  AND (NOT @cari_only::boolean OR left(l.account_code, 3) = ANY(@cari_codes::text[]))
  AND (
        (@term::text = '' AND j.entry_date BETWEEN @date_from AND @date_to
         AND abs((CASE WHEN @currency::text = 'TRY' THEN l.debit + l.credit ELSE COALESCE(l.foreign_amount, 0) END) - @amount::numeric) <= @tolerance::numeric)
     OR (@term::text <> '' AND (
            l.account_code LIKE @term::text || '%'
         OR l.description ILIKE '%' || @term::text || '%'
         OR j.description ILIKE '%' || @term::text || '%'
         OR COALESCE(l.document_no, '') ILIKE '%' || @term::text || '%'))
  )
ORDER BY j.entry_date DESC, j.entry_no DESC, l.line_no
LIMIT 50;

-- Elle eşleştirmede seçilen fiş satırları (anahtar "fiş uuid:satır no"). Süzgeçler
-- aramayla aynı; açık tutar bu hareketin eski eşleşmeleri hariç
-- (yeniden eşleştirme).
-- name: ListJournalLinesForManualMatch :many
SELECT l.id, l.line_no, l.account_code,
       (CASE WHEN @currency::text = 'TRY' THEN l.debit + l.credit ELSE COALESCE(l.foreign_amount, 0) END)::numeric AS line_amount,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = @taxpayer_id AND m.target_type = 'journal_line' AND m.target_id = l.id
                   AND m.bank_transaction_id <> @bank_transaction_id), 0)::numeric AS matched_amount,
       j.uuid AS journal_draft_uuid, j.entry_no, j.period
FROM journal_draft_lines l
JOIN journal_drafts j
  ON j.organization_id = l.organization_id AND j.engagement_id = l.engagement_id AND j.id = l.journal_draft_id
WHERE (j.uuid::text || ':' || l.line_no::text) = ANY(@keys::text[])
  AND l.organization_id = @organization_id AND l.engagement_id = @engagement_id
  AND j.organization_id = @organization_id AND j.engagement_id = @engagement_id AND j.taxpayer_id = @taxpayer_id
  AND l.source_kind IS DISTINCT FROM 'bank_transaction'
  AND (CASE WHEN @side::text = 'credit' THEN l.credit > 0 ELSE l.debit > 0 END)
  AND (@currency::text = 'TRY' OR (l.currency = @currency::text AND l.foreign_amount IS NOT NULL))
ORDER BY j.entry_date, j.entry_no, l.line_no;
