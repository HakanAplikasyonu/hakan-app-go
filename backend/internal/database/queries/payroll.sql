-- Personel, bordro ve SGK primleri (#382). Bütün tablolar mükellef
-- kapsamlı (taxpayer_tables.txt).

-- name: ListEmployees :many
-- status: active (çıkışı yok ya da bugünden sonra), exited, NULL = hepsi.
SELECT e.*, count(*) OVER ()::bigint AS total_count
FROM employees e
WHERE e.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'active' AND (e.exit_date IS NULL OR e.exit_date >= CURRENT_DATE))
       OR (sqlc.narg(status)::text = 'exited' AND e.exit_date < CURRENT_DATE))
  AND (sqlc.narg(q)::text IS NULL OR e.full_name ILIKE '%' || sqlc.narg(q)::text || '%'
       OR e.sgk_no ILIKE '%' || sqlc.narg(q)::text || '%' OR e.job_title ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY e.full_name, e.id
LIMIT @lim OFFSET @off;

-- name: CountEmployees :one
SELECT count(*) FILTER (WHERE exit_date IS NULL OR exit_date >= CURRENT_DATE)::bigint AS active,
       count(*) FILTER (WHERE exit_date < CURRENT_DATE)::bigint AS exited
FROM employees WHERE taxpayer_id = @taxpayer_id;

-- name: GetEmployeeByUUID :one
SELECT * FROM employees WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetEmployeeByDigest :one
SELECT * FROM employees WHERE taxpayer_id = @taxpayer_id AND national_id_digest = @national_id_digest;

-- name: FindEmployeeByName :one
-- TCKN'siz satır için: aynı adlı, TCKN'si bilinmeyen tek personel.
SELECT * FROM employees
WHERE taxpayer_id = @taxpayer_id AND national_id_digest IS NULL AND lower(full_name) = lower(@full_name::text)
ORDER BY id LIMIT 1;

-- name: InsertEmployee :one
INSERT INTO employees (taxpayer_id, full_name, national_id_masked, national_id_digest, sgk_no, job_title,
                       department, iban, hire_date, exit_date, exit_reason, created_by)
VALUES (@taxpayer_id, @full_name, sqlc.narg(national_id_masked), sqlc.narg(national_id_digest), sqlc.narg(sgk_no),
        sqlc.narg(job_title), sqlc.narg(department), sqlc.narg(iban), sqlc.narg(hire_date), sqlc.narg(exit_date),
        sqlc.narg(exit_reason), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateEmployee :one
UPDATE employees SET full_name = @full_name, national_id_masked = sqlc.narg(national_id_masked),
       national_id_digest = sqlc.narg(national_id_digest), sgk_no = sqlc.narg(sgk_no), job_title = sqlc.narg(job_title),
       department = sqlc.narg(department), iban = sqlc.narg(iban), hire_date = sqlc.narg(hire_date),
       exit_date = sqlc.narg(exit_date), exit_reason = sqlc.narg(exit_reason)
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: FillEmployeeFromImport :exec
-- İçe aktarma boş alanları doldurur, elle girileni ezmez.
UPDATE employees SET sgk_no = COALESCE(sgk_no, sqlc.narg(sgk_no)), job_title = COALESCE(job_title, sqlc.narg(job_title)),
       department = COALESCE(department, sqlc.narg(department)), hire_date = COALESCE(hire_date, sqlc.narg(hire_date)),
       exit_date = COALESCE(exit_date, sqlc.narg(exit_date))
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: GetPayrollRunByUUID :one
SELECT * FROM payroll_runs WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetPayrollRunByPeriod :one
SELECT * FROM payroll_runs WHERE taxpayer_id = @taxpayer_id AND period = @period;

-- name: UpsertPayrollRun :one
-- Aynı dönemin yeniden içe aktarılması toplamları ve kaynağı değiştirir.
INSERT INTO payroll_runs (taxpayer_id, period, source, source_system, source_file_name, employee_count, gross_total,
                          net_total, sgk_employee_total, unemployment_employee_total, sgk_employer_total,
                          unemployment_employer_total, sgk_incentive_total, income_tax_total, stamp_tax_total,
                          exemption_total, other_deductions_total, employer_cost_total, issues, created_by)
VALUES (@taxpayer_id, @period, @source, @source_system, sqlc.narg(source_file_name), @employee_count, @gross_total,
        @net_total, @sgk_employee_total, @unemployment_employee_total, @sgk_employer_total,
        @unemployment_employer_total, @sgk_incentive_total, @income_tax_total, @stamp_tax_total,
        @exemption_total, @other_deductions_total, @employer_cost_total, @issues, sqlc.narg(created_by))
ON CONFLICT (taxpayer_id, period) DO UPDATE SET
    source = EXCLUDED.source, source_system = EXCLUDED.source_system, source_file_name = EXCLUDED.source_file_name,
    employee_count = EXCLUDED.employee_count, gross_total = EXCLUDED.gross_total, net_total = EXCLUDED.net_total,
    sgk_employee_total = EXCLUDED.sgk_employee_total, unemployment_employee_total = EXCLUDED.unemployment_employee_total,
    sgk_employer_total = EXCLUDED.sgk_employer_total, unemployment_employer_total = EXCLUDED.unemployment_employer_total,
    sgk_incentive_total = EXCLUDED.sgk_incentive_total, income_tax_total = EXCLUDED.income_tax_total,
    stamp_tax_total = EXCLUDED.stamp_tax_total, exemption_total = EXCLUDED.exemption_total,
    other_deductions_total = EXCLUDED.other_deductions_total, employer_cost_total = EXCLUDED.employer_cost_total,
    issues = EXCLUDED.issues, created_by = EXCLUDED.created_by
RETURNING *;

-- name: DeletePayrollLines :exec
DELETE FROM payroll_lines WHERE taxpayer_id = @taxpayer_id AND payroll_run_id = @payroll_run_id;

-- name: InsertPayrollLine :exec
INSERT INTO payroll_lines (taxpayer_id, payroll_run_id, employee_id, line_no, employee_name, national_id_masked,
                           days_worked, gross, sgk_base, sgk_employee, unemployment_employee, income_tax_base,
                           income_tax, income_tax_exemption, stamp_tax, stamp_tax_exemption, agi, other_deductions,
                           net, sgk_employer, unemployment_employer, sgk_incentive, employer_cost)
VALUES (@taxpayer_id, @payroll_run_id, sqlc.narg(employee_id), @line_no, @employee_name, sqlc.narg(national_id_masked),
        sqlc.narg(days_worked), @gross, @sgk_base, @sgk_employee, @unemployment_employee, @income_tax_base,
        @income_tax, @income_tax_exemption, @stamp_tax, @stamp_tax_exemption, @agi, @other_deductions,
        @net, @sgk_employer, @unemployment_employer, @sgk_incentive, @employer_cost);

-- name: ListPayrollLines :many
SELECT l.*, e.uuid AS employee_uuid, e.iban AS employee_iban
FROM payroll_lines l
LEFT JOIN employees e ON e.id = l.employee_id AND e.taxpayer_id = l.taxpayer_id
WHERE l.taxpayer_id = @taxpayer_id AND l.payroll_run_id = @payroll_run_id
ORDER BY l.line_no;

-- name: ListPayrollRuns :many
-- Dönem bordroları, yeniden eskiye: net maaş ödemesine eşlenen banka
-- tutarı, SGK tahakkuk/ödeme durumu ve fiş taslağı.
SELECT r.*,
       COALESCE((SELECT sum(m.amount) FROM bank_transaction_matches m
                 WHERE m.taxpayer_id = r.taxpayer_id AND m.target_type = 'payroll_run' AND m.target_id = r.id), 0)::numeric(18,2) AS net_paid,
       s.uuid AS sgk_uuid, s.payable_amount AS sgk_payable, s.paid_amount AS sgk_paid, s.due_date AS sgk_due_date,
       jd.uuid AS journal_draft_uuid, jd.status AS journal_draft_status
FROM payroll_runs r
LEFT JOIN sgk_premiums s ON s.taxpayer_id = r.taxpayer_id AND s.period = r.period
LEFT JOIN journal_drafts jd ON jd.id = r.journal_draft_id AND jd.organization_id = @organization_id AND jd.engagement_id = @engagement_id
WHERE r.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(year)::text IS NULL OR left(r.period, 4) = sqlc.narg(year)::text)
ORDER BY r.period DESC
LIMIT @lim OFFSET @off;

-- name: PayrollRunNetPaid :one
SELECT COALESCE(sum(amount), 0)::numeric(18,2) AS net_paid
FROM bank_transaction_matches
WHERE taxpayer_id = @taxpayer_id AND target_type = 'payroll_run' AND target_id = @payroll_run_id;

-- name: SetPayrollRunJournalDraft :exec
UPDATE payroll_runs SET journal_draft_id = sqlc.narg(journal_draft_id)
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: GetPayrollJournalDraft :one
-- Bordronun bu ofisteki fiş taslağı ve durumu (taslak, onaylı, aktarılmış).
SELECT uuid, status FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: DeletePayrollRun :exec
DELETE FROM payroll_runs WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: UpsertSgkAccrual :one
-- Bordrodan gelen tahakkuk; ödeme bilgisi korunur.
INSERT INTO sgk_premiums (taxpayer_id, period, payroll_run_id, accrual_amount, incentive_amount, payable_amount, due_date)
VALUES (@taxpayer_id, @period, sqlc.narg(payroll_run_id), @accrual_amount, @incentive_amount, @payable_amount, @due_date)
ON CONFLICT (taxpayer_id, period) DO UPDATE SET
    payroll_run_id = EXCLUDED.payroll_run_id, accrual_amount = EXCLUDED.accrual_amount,
    incentive_amount = EXCLUDED.incentive_amount, payable_amount = EXCLUDED.payable_amount, due_date = EXCLUDED.due_date
RETURNING *;

-- name: ListSgkPremiums :many
SELECT s.*, bt.uuid AS bank_transaction_uuid
FROM sgk_premiums s
LEFT JOIN bank_transactions bt ON bt.id = s.bank_transaction_id AND bt.taxpayer_id = s.taxpayer_id
WHERE s.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(year)::text IS NULL OR left(s.period, 4) = sqlc.narg(year)::text)
ORDER BY s.period DESC
LIMIT @lim OFFSET @off;

-- name: GetSgkPremiumByUUID :one
SELECT * FROM sgk_premiums WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetSgkPremiumByPeriod :one
SELECT * FROM sgk_premiums WHERE taxpayer_id = @taxpayer_id AND period = @period;

-- name: InsertSgkPremium :one
-- Bordrosu olmayan dönem için elle girilen tahakkuk.
INSERT INTO sgk_premiums (taxpayer_id, period, accrual_amount, incentive_amount, payable_amount, due_date, document_no, note)
VALUES (@taxpayer_id, @period, @accrual_amount, @incentive_amount, @payable_amount, @due_date, sqlc.narg(document_no), sqlc.narg(note))
RETURNING *;

-- name: UpdateSgkPremium :one
UPDATE sgk_premiums SET accrual_amount = @accrual_amount, incentive_amount = @incentive_amount,
       payable_amount = @payable_amount, due_date = @due_date, document_no = sqlc.narg(document_no),
       paid_amount = @paid_amount, paid_at = sqlc.narg(paid_at), bank_transaction_id = sqlc.narg(bank_transaction_id),
       note = sqlc.narg(note)
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;

-- name: ListOpenSgkPremiums :many
SELECT * FROM sgk_premiums
WHERE taxpayer_id = @taxpayer_id AND paid_amount < payable_amount AND payable_amount > 0
ORDER BY period;

-- name: ListPayrollBankCandidates :many
-- Maaş ve SGK ödemesi olabilecek, eşleşmemiş çıkışlar.
SELECT bt.id, bt.uuid, bt.transaction_date, bt.amount, bt.currency, bt.kind, bt.kind_detail,
       bt.counterparty_name, bt.counterparty_iban, bt.description
FROM bank_transactions bt
WHERE bt.taxpayer_id = @taxpayer_id AND NOT bt.is_cancelled AND bt.debit_credit = 'B' AND bt.currency = 'TRY'
  AND bt.match_status IN ('unmatched', 'suggested')
  AND bt.transaction_date BETWEEN @date_from AND @date_to
ORDER BY bt.transaction_date, bt.id;

-- name: InsertPayrollDocument :one
INSERT INTO payroll_documents (taxpayer_id, period, payroll_run_id, employee_id, kind, title, storage_key, sha256,
                               size_bytes, content_type, file_name, visible_to_taxpayer, created_by)
VALUES (@taxpayer_id, @period, sqlc.narg(payroll_run_id), sqlc.narg(employee_id), @kind, @title, @storage_key, @sha256,
        @size_bytes, @content_type, @file_name, @visible_to_taxpayer, sqlc.narg(created_by))
ON CONFLICT (taxpayer_id, sha256) DO NOTHING
RETURNING *;

-- name: ListPayrollDocuments :many
SELECT d.*, e.full_name AS employee_name, e.uuid AS employee_uuid, count(*) OVER ()::bigint AS total_count
FROM payroll_documents d
LEFT JOIN employees e ON e.id = d.employee_id AND e.taxpayer_id = d.taxpayer_id
WHERE d.taxpayer_id = @taxpayer_id
  AND (NOT @only_visible::boolean OR d.visible_to_taxpayer)
  AND (sqlc.narg(period)::text IS NULL OR d.period = sqlc.narg(period)::text)
ORDER BY d.period DESC, d.created_at DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: GetPayrollDocument :one
SELECT d.*, e.full_name AS employee_name, e.uuid AS employee_uuid
FROM payroll_documents d
LEFT JOIN employees e ON e.id = d.employee_id AND e.taxpayer_id = d.taxpayer_id
WHERE d.taxpayer_id = @taxpayer_id AND d.uuid = @uuid
  AND (NOT @only_visible::boolean OR d.visible_to_taxpayer);

-- name: DeletePayrollDocument :exec
DELETE FROM payroll_documents WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: GetJournalDraftIDByUUID :one
SELECT id FROM journal_drafts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: CountBankTransactionMatches :one
SELECT count(*)::bigint FROM bank_transaction_matches
WHERE taxpayer_id = @taxpayer_id AND bank_transaction_id = @bank_transaction_id;

-- name: DeleteTargetMatches :many
-- Bordro ya da SGK tahakkuku silinince eşleşmeleri de silinir.
DELETE FROM bank_transaction_matches
WHERE taxpayer_id = @taxpayer_id AND target_type = @target_type AND target_id = @target_id
RETURNING bank_transaction_id;
