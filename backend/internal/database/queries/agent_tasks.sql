-- Masaüstü yardımcısı görevleri (docs/tasarim/masaustu-luca-sgk.md §2).

-- name: GetDesktopHelperSettings :one
SELECT * FROM desktop_helper_settings WHERE organization_id = @organization_id;

-- name: UpsertDesktopHelperSettings :one
INSERT INTO desktop_helper_settings (organization_id, luca_enabled, sgk_enabled, updated_by)
VALUES (@organization_id, @luca_enabled, @sgk_enabled, sqlc.narg(updated_by))
ON CONFLICT (organization_id) DO UPDATE
SET luca_enabled = EXCLUDED.luca_enabled, sgk_enabled = EXCLUDED.sgk_enabled, updated_by = EXCLUDED.updated_by
RETURNING *;

-- tenant-lint: org-wide görev açılacak mükellefler; read_all yoksa engagement_assignments ile süzülür
-- name: ListHelperEngagements :many
-- Görev açılacak mükellefler: ofisin, kullanıcının görebildiği, etkin.
SELECT e.id, e.uuid, e.organization_id, e.taxpayer_id, e.status, e.started_at,
       t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, COALESCE(t.tax_id, '')::text AS taxpayer_tax_id
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.uuid = ANY(@uuids::uuid[]) AND e.status = 'active'
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
ORDER BY t.title, e.id;

-- name: InsertBulkAgentTask :one
INSERT INTO agent_tasks (organization_id, engagement_id, kind, run_uuid, sequence, period, kinds,
                         requested_by, approved_by, approved_at)
VALUES (@organization_id, @engagement_id, 'luca_bulk_export', @run_uuid, @sequence, @period, @kinds,
        @requested_by, @requested_by, NOW())
ON CONFLICT (engagement_id, period) WHERE kind = 'luca_bulk_export' AND status IN ('queued', 'claimed', 'waiting_user', 'running')
DO NOTHING
RETURNING *;

-- name: EnsureCariAgentTask :one
-- Mükellefin onaysız cari görevi: yoksa açılır, varsa o döner.
INSERT INTO agent_tasks (organization_id, engagement_id, kind, requested_by)
VALUES (@organization_id, @engagement_id, 'luca_cari_sync', sqlc.narg(requested_by))
ON CONFLICT (engagement_id) WHERE kind = 'luca_cari_sync' AND status = 'queued' AND approved_at IS NULL
DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: AddAgentTaskCounterparty :execrows
-- VKN + ana hesap başına tek satır; aynı cari ikinci kez eklenmez.
INSERT INTO agent_task_counterparties (organization_id, engagement_id, task_id, tax_id, main_code, title)
VALUES (@organization_id, @engagement_id, @task_id, @tax_id, @main_code, @title)
ON CONFLICT (task_id, tax_id, main_code) DO NOTHING;

-- name: ListMissingCounterparties :many
-- Dönemin faturalarında cari hafızasında olmayan karşı taraflar: gelen
-- fatura satıcıdır (320), giden fatura alıcıdır (120).
SELECT DISTINCT ON (x.tax_id, x.main_code) x.tax_id::text AS tax_id, x.main_code::text AS main_code, x.title::text AS title
FROM (
    SELECT COALESCE(NULLIF(i.customer_tax_number, ''), i.customer_identity_number) AS tax_id,
           CASE WHEN i.direction = 'incoming' THEN '320' ELSE '120' END AS main_code,
           COALESCE(i.customer_name, '') AS title, i.id
    FROM invoices i
    WHERE i.taxpayer_id = @taxpayer_id AND i.period = @period
) x
WHERE x.tax_id ~ '^[0-9]{10,11}$'
  AND NOT EXISTS (
    SELECT 1 FROM counterparty_accounts c
    WHERE c.organization_id = @organization_id AND c.engagement_id = @engagement_id
      AND c.counterparty_tax_id = x.tax_id AND c.main_code = x.main_code)
ORDER BY x.tax_id, x.main_code, x.id
LIMIT 500;

-- name: CountAgentTaskCounterparties :one
SELECT COUNT(*)::bigint FROM agent_task_counterparties
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id;

-- name: AttachCariAgentTaskToRun :one
-- Bekleyen cari görevi aktarım sırasına girer: onaylanır, sırada önce gelir.
UPDATE agent_tasks
SET run_uuid = @run_uuid, sequence = @sequence, approved_by = @approved_by, approved_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND kind = 'luca_cari_sync' AND status = 'queued' AND approved_at IS NULL
RETURNING *;

-- name: DeleteEmptyCariAgentTask :exec
-- Satırı olmayan cari görevi kalmaz.
DELETE FROM agent_tasks x
WHERE x.organization_id = @organization_id AND x.engagement_id = @engagement_id AND x.id = @id
  AND x.kind = 'luca_cari_sync' AND x.status = 'queued' AND x.approved_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM agent_task_counterparties c
                  WHERE c.organization_id = x.organization_id AND c.engagement_id = x.engagement_id AND c.task_id = x.id);

-- tenant-lint: org-wide görev ayrıntısı uuid ile; read_all yoksa engagement_assignments ile süzülür
-- name: GetAgentTask :one
SELECT x.*, e.uuid AS engagement_uuid, e.taxpayer_id, e.status AS engagement_status, e.started_at AS engagement_started_at,
       t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, COALESCE(t.tax_id, '')::text AS taxpayer_tax_id,
       a.uuid AS agent_uuid, a.name AS agent_name,
       btrim(COALESCE(ru.name, '') || ' ' || COALESCE(ru.surname, ''))::text AS requested_by_name,
       btrim(COALESCE(au.name, '') || ' ' || COALESCE(au.surname, ''))::text AS approved_by_name
FROM agent_tasks x
JOIN engagements e ON e.organization_id = x.organization_id AND e.id = x.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
LEFT JOIN desktop_agents a ON a.id = x.agent_id AND a.organization_id = x.organization_id
LEFT JOIN users ru ON ru.id = x.requested_by
LEFT JOIN users au ON au.id = x.approved_by
WHERE x.organization_id = @organization_id AND x.uuid = @uuid
  AND (@all_engagements::bool OR x.engagement_id IN (
        SELECT g.engagement_id FROM engagement_assignments g
        WHERE g.organization_id = @organization_id AND g.user_id = @user_id));

-- tenant-lint: org-wide ofis görev listesi; read_all yoksa engagement_assignments ile süzülür
-- name: ListAgentTasks :many
SELECT x.*, e.uuid AS engagement_uuid, e.taxpayer_id, e.status AS engagement_status, e.started_at AS engagement_started_at,
       t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, COALESCE(t.tax_id, '')::text AS taxpayer_tax_id,
       a.uuid AS agent_uuid, a.name AS agent_name,
       btrim(COALESCE(ru.name, '') || ' ' || COALESCE(ru.surname, ''))::text AS requested_by_name,
       btrim(COALESCE(au.name, '') || ' ' || COALESCE(au.surname, ''))::text AS approved_by_name,
       count(*) OVER ()::bigint AS total
FROM agent_tasks x
JOIN engagements e ON e.organization_id = x.organization_id AND e.id = x.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
LEFT JOIN desktop_agents a ON a.id = x.agent_id AND a.organization_id = x.organization_id
LEFT JOIN users ru ON ru.id = x.requested_by
LEFT JOIN users au ON au.id = x.approved_by
WHERE x.organization_id = @organization_id
  AND (@all_engagements::bool OR x.engagement_id IN (
        SELECT g.engagement_id FROM engagement_assignments g
        WHERE g.organization_id = @organization_id AND g.user_id = @user_id))
  AND (sqlc.narg(status)::text IS NULL OR x.status = sqlc.narg(status))
  AND (NOT @open_only::bool OR x.status IN ('queued', 'claimed', 'waiting_user', 'running'))
  AND (sqlc.narg(kind)::text IS NULL OR x.kind = sqlc.narg(kind))
  AND (sqlc.narg(run_uuid)::uuid IS NULL OR x.run_uuid = sqlc.narg(run_uuid))
  AND (sqlc.narg(engagement_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(engagement_uuid))
ORDER BY x.created_at DESC, x.run_uuid, x.sequence, x.id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide listedeki görevlerin dosyaları; görevler zaten süzüldü
-- name: ListAgentTaskFiles :many
SELECT * FROM agent_task_files
WHERE organization_id = @organization_id AND task_id = ANY(@task_ids::bigint[])
ORDER BY task_id, sequence;

-- tenant-lint: org-wide listedeki görevlerin carileri; görevler zaten süzüldü
-- name: ListAgentTaskCounterparties :many
SELECT * FROM agent_task_counterparties
WHERE organization_id = @organization_id AND task_id = ANY(@task_ids::bigint[])
ORDER BY task_id, main_code, title, tax_id;

-- tenant-lint: org-wide ajanın üstünde açık görev var mı
-- name: CountAgentBusyTasks :one
SELECT COUNT(*)::bigint FROM agent_tasks
WHERE organization_id = @organization_id AND agent_id = @agent_id AND id <> @id
  AND status IN ('claimed', 'waiting_user', 'running');

-- tenant-lint: org-wide bir iş sırası birden çok mükellefi kapsar
-- name: CountRunBlockers :one
-- Aynı sırada önce gelen ve bitmemiş görevler (iş sırası).
SELECT COUNT(*)::bigint FROM agent_tasks
WHERE organization_id = @organization_id AND run_uuid = @run_uuid AND sequence < @sequence
  AND status IN ('queued', 'claimed', 'waiting_user', 'running');

-- name: ClaimAgentTask :one
-- Onaylı bekleyen görevi ajan üstlenir; aynı ajan tekrar üstlenirse aynı görev.
UPDATE agent_tasks
SET status = CASE WHEN status = 'queued' THEN 'claimed' ELSE status END,
    agent_id = @agent_id, claimed_at = COALESCE(claimed_at, NOW())
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND approved_at IS NOT NULL
  AND (status = 'queued' OR (status IN ('claimed', 'waiting_user', 'running') AND agent_id = @agent_id))
RETURNING *;

-- name: SetAgentTaskProgress :one
UPDATE agent_tasks
SET status = @status, progress = @progress
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND agent_id = @agent_id AND status IN ('claimed', 'waiting_user', 'running')
RETURNING *;

-- name: SetAgentTaskServerReport :exec
UPDATE agent_tasks SET server_report = @server_report
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: FinishAgentTask :one
-- Ajanın son sonucu.
UPDATE agent_tasks
SET status = @status, error_code = sqlc.narg(error_code), error_message = sqlc.narg(error_message), finished_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND agent_id = @agent_id AND status IN ('claimed', 'waiting_user', 'running')
RETURNING *;

-- name: FinishAgentTaskBySystem :one
-- Üretilecek belge yoksa görev ajan beklemeden biter.
UPDATE agent_tasks
SET status = @status, error_code = sqlc.narg(error_code), error_message = sqlc.narg(error_message), finished_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND status IN ('claimed', 'waiting_user', 'running')
RETURNING *;

-- name: ApproveAgentTask :one
UPDATE agent_tasks SET approved_by = @approved_by, approved_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND status = 'queued' AND approved_at IS NULL
RETURNING *;

-- name: CancelAgentTask :one
UPDATE agent_tasks SET status = 'cancelled', resolved_by = @resolved_by, finished_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND status IN ('queued', 'claimed', 'waiting_user')
RETURNING *;

-- name: ResolveAgentTask :one
-- needs_check görevini kullanıcı Luca'ya bakıp kapatır.
UPDATE agent_tasks SET resolution = @resolution, resolved_by = @resolved_by
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND status = 'needs_check' AND resolution IS NULL
RETURNING *;

-- name: InsertAgentTaskFile :one
INSERT INTO agent_task_files (organization_id, engagement_id, task_id, sequence, kind, name, content_type,
                              size, sha256, storage_key, entries, export_uuid, transfer_id, documents)
VALUES (@organization_id, @engagement_id, @task_id, @sequence, @kind, @name, @content_type,
        @size, @sha256, @storage_key, @entries, sqlc.narg(export_uuid), sqlc.narg(transfer_id), @documents)
RETURNING *;

-- name: GetAgentTaskFile :one
SELECT * FROM agent_task_files
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id AND uuid = @uuid;

-- name: ReportAgentTaskFile :one
UPDATE agent_task_files
SET status = @status, luca_message = sqlc.narg(luca_message), entry_results = @entry_results, reported_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id AND uuid = @uuid
  AND status = 'pending'
RETURNING *;

-- name: ListTaskFilesOfTask :many
SELECT * FROM agent_task_files
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id
ORDER BY sequence;

-- name: ListTaskCounterpartiesOfTask :many
SELECT * FROM agent_task_counterparties
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id
ORDER BY id;

-- name: SetAgentTaskCounterpartyResult :execrows
UPDATE agent_task_counterparties
SET status = @status, account_code = sqlc.narg(account_code), hakan_account_code = sqlc.narg(hakan_account_code),
    message = sqlc.narg(message), title = CASE WHEN @title::text = '' THEN title ELSE @title::text END
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND task_id = @task_id
  AND tax_id = @tax_id AND main_code = @main_code;

-- name: GetCounterpartyAccountCode :one
SELECT account_code FROM counterparty_accounts
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND counterparty_tax_id = @tax_id AND main_code = @main_code;

-- name: InsertCounterpartyAccountFromLuca :execrows
-- Luca'da bulunan / açılan cari kodu hafızaya yazılır; var olan karar ezilmez.
INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code,
                                   account_code, account_name, decided_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @tax_id, @main_code, @account_code, @account_name, sqlc.narg(decided_by))
ON CONFLICT (engagement_id, counterparty_tax_id, main_code) DO NOTHING;

-- name: ListLucaBatchesWithoutTransfer :many
-- Dönemin aktarılmamış Luca yevmiye çıktıları (yalnız geri alınmamış fişi olan).
SELECT b.* FROM export_batches b
WHERE b.organization_id = @organization_id AND b.engagement_id = @engagement_id
  AND b.driver = 'luca' AND b.period = @period
  AND NOT EXISTS (
    SELECT 1 FROM accounting_transfers t
    WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id
      AND t.export_batch_id = b.id AND t.status IN ('queued', 'claimed', 'delivered', 'imported'))
  AND EXISTS (
    SELECT 1 FROM export_batch_items i
    WHERE i.organization_id = @organization_id AND i.engagement_id = @engagement_id
      AND i.export_batch_id = b.id AND i.reversed_at IS NULL)
ORDER BY b.id;

-- name: InsertLucaWebTransfer :one
-- Yevmiye çıktısı yardımcıyla Luca'ya; satır dönmezse çıktının açık ya da
-- aktarılmış görevi var. Görev ajanındadır (claimed).
INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, method, status,
                                  agent_id, requested_by, claimed_at)
VALUES (@organization_id, @engagement_id, @export_batch_id, 'luca', 'luca_web', 'claimed',
        sqlc.narg(agent_id), @requested_by, NOW())
ON CONFLICT (export_batch_id) WHERE status IN ('queued', 'claimed', 'delivered', 'imported') DO NOTHING
RETURNING *;

-- name: FinishLucaWebTransfer :execrows
-- Yardımcının dosya sonucu: aktarıldı ya da hata verdi.
UPDATE accounting_transfers
SET status = @status, completed_by = sqlc.narg(completed_by), completed_at = NOW(),
    error_code = sqlc.narg(error_code), error_message = sqlc.narg(error_message)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND method = 'luca_web' AND status = 'claimed';

-- name: ListLucaListExportFileDocuments :many
-- Bir liste / masraf dosyasındaki belgeler (hatalı satırı belgeye eşlemek için).
SELECT document_key, COALESCE(document_no, '')::text AS document_no, document_date, file_name
FROM expense_export_documents
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND export_uuid = @export_uuid
ORDER BY id;

-- tenant-lint: cross-org yeni fatura mükellefin etkin dönemine ve yardımcısı açık ofise bağlanır
-- name: EnqueueCariForInvoice :execrows
-- Yeni cari tetiği: fatura gelince karşı tarafın VKN / TCKN'si mükellefin
-- cari hafızasında yoksa ve ofiste yardımcı açıksa onaysız cari görevine
-- eklenir (yoksa açılır). Etkin dönemi olmayan mükellefte bir şey olmaz.
WITH eng AS (
    SELECT e.organization_id, e.id AS engagement_id
    FROM engagements e
    JOIN desktop_helper_settings s ON s.organization_id = e.organization_id AND s.luca_enabled
    WHERE e.taxpayer_id = @taxpayer_id AND e.status = 'active'
      AND NOT EXISTS (
        SELECT 1 FROM counterparty_accounts c
        WHERE c.organization_id = e.organization_id AND c.engagement_id = e.id
          AND c.counterparty_tax_id = @tax_id::text AND c.main_code = @main_code::text)
), task AS (
    INSERT INTO agent_tasks (organization_id, engagement_id, kind)
    SELECT organization_id, engagement_id, 'luca_cari_sync' FROM eng
    ON CONFLICT (engagement_id) WHERE kind = 'luca_cari_sync' AND status = 'queued' AND approved_at IS NULL
    DO UPDATE SET updated_at = NOW()
    RETURNING id, organization_id, engagement_id
)
INSERT INTO agent_task_counterparties (organization_id, engagement_id, task_id, tax_id, main_code, title)
SELECT organization_id, engagement_id, id, @tax_id::text, @main_code::text, @title::text FROM task
ON CONFLICT (task_id, tax_id, main_code) DO NOTHING;
