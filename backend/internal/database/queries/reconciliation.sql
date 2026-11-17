-- Mutabakat (#389): mizan – kaynak çifte mutabakatı. Çalıştırma, kalem ve
-- cari tabloları engagement kapsamlı (tenant_tables.txt); kaynak tablolar
-- (fatura, banka, masraf, Z, bordro) mükellef kapsamlı.

-- name: ReconciliationJournalTotals :many
-- Dönem aralığındaki mahsup fişlerinin satır toplamları; hesap, kaynak türü
-- ve taslak (aktarılmamış) olup olmadığına göre.
SELECT l.account_code::text AS account_code,
       COALESCE(l.source_kind, '')::text AS source_kind,
       (COALESCE(l.source_id, '') LIKE 'retail_sale:%')::bool AS is_retail,
       (d.status = 'draft')::bool AS is_draft,
       COALESCE(SUM(l.debit), 0)::numeric AS debit,
       COALESCE(SUM(l.credit), 0)::numeric AS credit,
       COUNT(*)::bigint AS line_count
FROM journal_draft_lines l
JOIN journal_drafts d ON d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
  AND d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND d.entry_type = 'mahsup'
  AND d.period >= @window_from::text AND d.period <= @window_to::text
GROUP BY 1, 2, 3, 4
ORDER BY 1, 2, 3, 4;

-- name: ReconciliationUnpostedBank :many
-- Fişe girmemiş banka hareketleri (iptal edilenler hariç).
SELECT t.id, t.uuid, t.transaction_date, t.debit_credit::text AS debit_credit, t.amount, t.currency::text AS currency,
       COALESCE(NULLIF(t.counterparty_name, ''), t.description, '')::text AS label
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id
  AND t.transaction_date >= @date_from::date AND t.transaction_date <= @date_to::date
  AND NOT t.is_cancelled
  AND NOT EXISTS (
      SELECT 1 FROM journal_draft_lines l
      WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
        AND l.source_kind = 'bank_transaction' AND l.source_id = t.id::text)
ORDER BY t.transaction_date, t.id
LIMIT 1000;

-- name: ReconciliationUnpostedInvoices :many
-- Fişe girmemiş (ne faturanın ne kaleminin satırı olan) etkin faturalar.
SELECT i.id, i.uuid, i.invoice_number::text AS invoice_number, i.issue_date, i.direction, i.type,
       i.currency::text AS currency, i.exchange_rate,
       i.tax_exclusive_amount, i.tax_amount, i.payable_amount, i.payable_amount_try,
       COALESCE(i.customer_name, '')::text AS counterparty_name,
       COALESCE(i.customer_tax_number, i.customer_identity_number, '')::text AS counterparty_tax_id
FROM invoices i
WHERE i.taxpayer_id = @taxpayer_id
  AND i.period >= @window_from::text AND i.period <= @window_to::text
  AND i.status_normalized = 'active'
  AND NOT EXISTS (
      SELECT 1 FROM journal_draft_lines l
      WHERE l.organization_id = @organization_id AND l.engagement_id = @engagement_id
        AND ((l.source_kind = 'invoice' AND l.source_id = i.id::text)
          OR (l.source_kind = 'invoice_item' AND l.source_id IN (
                SELECT it.id::text FROM invoice_items it WHERE it.taxpayer_id = @taxpayer_id AND it.invoice_id = i.id))))
ORDER BY i.issue_date, i.id
LIMIT 2000;

-- name: ReconciliationUnpostedExpenses :many
-- Reddedilmemiş, yevmiye taslağı olmayan masraflar (Z raporu değil).
SELECT e.id, e.uuid, e.expense_date, e.amount, e.tax_amount, e.currency::text AS currency,
       COALESCE(e.vendor_name, e.description, '')::text AS label,
       COALESCE(p.account_code, '')::text AS account_code
FROM expenses e
LEFT JOIN receipt_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
      AND p.target_type = 'expense' AND p.target_id = e.id
WHERE e.taxpayer_id = @taxpayer_id
  AND e.period >= @window_from::text AND e.period <= @window_to::text
  AND e.approval_status <> 'rejected'
  AND COALESCE(e.receipt_type, '') <> 'z_report'
  AND p.journal_draft_id IS NULL
ORDER BY e.expense_date, e.id
LIMIT 1000;

-- name: ReconciliationUnpostedRetailSales :many
-- Reddedilmemiş, yevmiye taslağı olmayan Z raporları.
SELECT r.id, r.uuid, r.sale_date, r.subtotal_amount, r.tax_amount, r.total_amount,
       COALESCE(r.z_report_number, '')::text AS z_report_number
FROM retail_sales r
LEFT JOIN receipt_postings p
       ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
      AND p.target_type = 'retail_sale' AND p.target_id = r.id
WHERE r.taxpayer_id = @taxpayer_id
  AND r.period >= @window_from::text AND r.period <= @window_to::text
  AND r.approval_status <> 'rejected'
  AND p.journal_draft_id IS NULL
ORDER BY r.sale_date, r.id
LIMIT 1000;

-- name: ReconciliationUnpostedPayrollRuns :many
-- Fiş taslağı olmayan bordrolar.
SELECT r.id, r.uuid, r.period::text AS period, r.net_total, r.employer_cost_total,
       r.gross_total, r.sgk_employer_total, r.unemployment_employer_total, r.sgk_incentive_total
FROM payroll_runs r
WHERE r.taxpayer_id = @taxpayer_id
  AND r.period >= @window_from::text AND r.period <= @window_to::text
  AND r.journal_draft_id IS NULL
ORDER BY r.period, r.id;

-- name: ReconciliationBankClosingBalances :many
-- Her banka hesabının (cash box) dönem sonuna kadarki son hareketinin
-- bakiyesi; bakiyesi okunmamış hesap sayılmaz.
SELECT DISTINCT ON (t.cash_box_id) t.cash_box_id, t.transaction_date, t.balance_after::numeric AS balance_after,
       t.currency::text AS currency
FROM bank_transactions t
WHERE t.taxpayer_id = @taxpayer_id AND t.transaction_date <= @date_to::date
  AND t.balance_after IS NOT NULL AND NOT t.is_cancelled
ORDER BY t.cash_box_id, t.transaction_date DESC, t.bank_sequence DESC NULLS LAST, t.id DESC;

-- name: InsertReconciliationRun :one
INSERT INTO reconciliation_runs (organization_id, engagement_id, taxpayer_id, period, trial_balance_import_id,
    previous_import_id, window_from, window_to, tolerance, summary, created_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, sqlc.narg(trial_balance_import_id),
    sqlc.narg(previous_import_id), @window_from, @window_to, @tolerance, @summary, sqlc.narg(created_by))
RETURNING *;

-- name: InsertReconciliationItems :execrows
-- rows: [{"main_code","account_name","ledger_debit",...,"status","note"}]
INSERT INTO reconciliation_items (organization_id, engagement_id, run_id, main_code, account_name,
    ledger_debit, ledger_credit, source_debit, source_credit, difference, result, status,
    sources, unposted, sub_accounts, suggestion, suggestion_code, suggestion_confidence, note)
SELECT @organization_id, @engagement_id, @run_id, a.main_code, a.account_name,
       a.ledger_debit, a.ledger_credit, a.source_debit, a.source_credit, a.difference, a.result, a.status,
       a.sources, a.unposted, a.sub_accounts, a.suggestion, a.suggestion_code, a.suggestion_confidence, a.note
FROM jsonb_to_recordset(@rows::jsonb) AS a(main_code text, account_name text,
    ledger_debit numeric, ledger_credit numeric, source_debit numeric, source_credit numeric, difference numeric,
    result text, status text, sources jsonb, unposted jsonb, sub_accounts jsonb,
    suggestion text, suggestion_code text, suggestion_confidence numeric, note text);

-- name: ListReconciliationRuns :many
SELECT r.*, i.uuid AS trial_balance_uuid, i.file_name AS trial_balance_file_name
FROM reconciliation_runs r
LEFT JOIN account_plan_imports i ON i.organization_id = r.organization_id AND i.id = r.trial_balance_import_id
WHERE r.organization_id = @organization_id AND r.engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR r.period = sqlc.narg(period)::text)
ORDER BY r.period DESC, r.id DESC
LIMIT @lim;

-- name: GetReconciliationRun :one
SELECT r.*, i.uuid AS trial_balance_uuid, i.file_name AS trial_balance_file_name
FROM reconciliation_runs r
LEFT JOIN account_plan_imports i ON i.organization_id = r.organization_id AND i.id = r.trial_balance_import_id
WHERE r.organization_id = @organization_id AND r.engagement_id = @engagement_id AND r.uuid = @uuid;

-- name: LatestReconciliationRun :one
-- Dönemin en son çalıştırması (önceki karar ve notları taşımak için).
SELECT * FROM reconciliation_runs
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period::text
ORDER BY id DESC
LIMIT 1;

-- name: ListReconciliationItems :many
-- Bir çalıştırmanın kalemleri; inceleme kuyruğundaki önerinin durumuyla.
SELECT it.*, s.uuid AS review_uuid, s.status AS review_status
FROM reconciliation_items it
LEFT JOIN suggestions s ON s.organization_id = it.organization_id AND s.id = it.review_suggestion_id
WHERE it.organization_id = @organization_id AND it.engagement_id = @engagement_id AND it.run_id = @run_id
ORDER BY it.main_code;

-- name: GetReconciliationItem :one
SELECT it.*, s.uuid AS review_uuid, s.status AS review_status, r.period AS run_period, r.uuid AS run_uuid
FROM reconciliation_items it
JOIN reconciliation_runs r ON r.engagement_id = it.engagement_id AND r.id = it.run_id
LEFT JOIN suggestions s ON s.organization_id = it.organization_id AND s.id = it.review_suggestion_id
WHERE it.organization_id = @organization_id AND it.engagement_id = @engagement_id AND it.uuid = @uuid;

-- name: GetReconciliationItemByID :one
SELECT * FROM reconciliation_items
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
FOR UPDATE;

-- name: UpdateReconciliationItemDecision :one
UPDATE reconciliation_items
SET status = @status, note = sqlc.narg(note), decided_by = sqlc.narg(decided_by), decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: SetReconciliationItemReview :exec
UPDATE reconciliation_items
SET status = 'in_review', review_suggestion_id = @review_suggestion_id, decided_by = sqlc.narg(decided_by), decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;
