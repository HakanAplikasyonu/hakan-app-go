-- Muhasebe programına aktarım görevleri (issue #391).

-- name: InsertAccountingTransfer :one
-- Satır dönmezse çıktının açık ya da aktarılmış görevi var.
INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, method, requested_by)
VALUES (@organization_id, @engagement_id, @export_batch_id, @driver, @method, @requested_by)
ON CONFLICT (export_batch_id) WHERE status IN ('queued', 'claimed', 'delivered', 'imported') DO NOTHING
RETURNING *;

-- name: InsertManualAccountingTransfer :one
-- "Luca'ya yükledim" (#780): indirilen çıktı programa elle yüklendi; görev
-- doğrudan aktarıldı olarak açılır, kim ve ne zaman ile. Satır dönmezse
-- çıktının açık ya da aktarılmış görevi var.
INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, method, status,
                                  requested_by, completed_by, completed_at)
VALUES (@organization_id, @engagement_id, @export_batch_id, @driver, 'manual', 'imported',
        @requested_by, @requested_by, NOW())
ON CONFLICT (export_batch_id) WHERE status IN ('queued', 'claimed', 'delivered', 'imported') DO NOTHING
RETURNING *;

-- name: GetAccountingTransfer :one
SELECT t.*, b.uuid AS batch_uuid, b.period AS batch_period, b.files AS batch_files,
       a.uuid AS agent_uuid, a.name AS agent_name,
       btrim(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS completed_by_name
FROM accounting_transfers t
JOIN export_batches b ON b.id = t.export_batch_id AND b.engagement_id = t.engagement_id
LEFT JOIN desktop_agents a ON a.id = t.agent_id AND a.organization_id = t.organization_id
LEFT JOIN users u ON u.id = t.completed_by
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id AND t.uuid = @uuid;

-- name: ListAccountingTransfers :many
SELECT t.*, b.uuid AS batch_uuid, b.period AS batch_period, b.files AS batch_files,
       a.uuid AS agent_uuid, a.name AS agent_name,
       btrim(COALESCE(u.name, '') || ' ' || COALESCE(u.surname, ''))::text AS completed_by_name
FROM accounting_transfers t
JOIN export_batches b ON b.id = t.export_batch_id AND b.engagement_id = t.engagement_id
LEFT JOIN desktop_agents a ON a.id = t.agent_id AND a.organization_id = t.organization_id
LEFT JOIN users u ON u.id = t.completed_by
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
ORDER BY t.created_at DESC, t.id DESC
LIMIT @lim OFFSET @off;

-- name: CountAccountingTransfers :one
SELECT COUNT(*)::bigint FROM accounting_transfers t
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status));

-- name: ClaimAccountingTransfer :one
-- Ajan görevi üstlenir. Bekleyen ya da 15 dakikadır sonuç bildirilmemiş
-- (ajan kapandı) görev üstlenilebilir; iki ajan aynı görevi alamaz.
UPDATE accounting_transfers
SET status = 'claimed', agent_id = @agent_id, claimed_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND (status = 'queued'
       OR (status = 'claimed' AND (agent_id = @agent_id OR claimed_at < NOW() - INTERVAL '15 minutes')))
RETURNING *;

-- name: DeliverAccountingTransfer :one
-- Ajanın sonucu: dosyalar klasörde (delivered) ya da hata (failed).
UPDATE accounting_transfers
SET status = @status, delivered_files = @delivered_files, folder_label = sqlc.narg(folder_label),
    error_code = sqlc.narg(error_code), error_message = sqlc.narg(error_message),
    delivered_at = CASE WHEN @status = 'delivered' THEN NOW() ELSE delivered_at END,
    completed_at = CASE WHEN @status = 'failed' THEN NOW() ELSE completed_at END
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status = 'claimed' AND agent_id = @agent_id
RETURNING *;

-- name: CompleteAccountingTransfer :one
-- Kullanıcının kapanışı: programda aktarıldı (imported) ya da program hata
-- verdi (failed, mesajıyla). Yalnızca teslim edilmiş görev kapanır.
UPDATE accounting_transfers
SET status = @status, completed_by = @completed_by, completed_at = NOW(),
    error_code = sqlc.narg(error_code), error_message = sqlc.narg(error_message)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status = 'delivered'
RETURNING *;

-- name: CancelAccountingTransfer :one
UPDATE accounting_transfers
SET status = 'cancelled', completed_by = @completed_by, completed_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status IN ('queued', 'claimed', 'delivered')
RETURNING *;

-- name: GetActiveDesktopAgent :one
SELECT * FROM desktop_agents
WHERE organization_id = @organization_id AND uuid = @uuid AND revoked_at IS NULL;

-- tenant-lint: org-wide ofis bekleyen aktarımlar; read_all yoksa engagement_assignments ile süzülür
-- name: ListOfficePendingTransfers :many
-- Ofis genelinde programa henüz aktarılmamış onaylı fişler (#574), mükellef
-- ve dönem bazında: approved = onaylı, çıktısı alınmamış; in_transfer =
-- çıktısının son aktarımı sürüyor (kuyrukta, ajanda, klasörde); failed =
-- son aktarımı hata verdi. Yalnız indirilen ya da aktarımı iptal edilen
-- çıktılar sayılmaz (programa elle alınmış olabilir). Eski dönem önce.
WITH visible AS (
    SELECT e.id, e.uuid, t.title
    FROM engagements e
    JOIN taxpayers t ON t.id = e.taxpayer_id
    WHERE e.organization_id = @organization_id
      AND (@all_engagements::bool OR e.id IN (
            SELECT a.engagement_id FROM engagement_assignments a
            WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
), approved AS (
    SELECT d.engagement_id, d.period, count(*)::bigint AS n
    FROM journal_drafts d
    WHERE d.organization_id = @organization_id AND d.status = 'approved'
      AND d.engagement_id IN (SELECT id FROM visible)
    GROUP BY d.engagement_id, d.period
), latest AS (
    SELECT DISTINCT ON (x.export_batch_id) x.engagement_id, x.export_batch_id, x.status
    FROM accounting_transfers x
    WHERE x.organization_id = @organization_id AND x.engagement_id IN (SELECT id FROM visible)
    ORDER BY x.export_batch_id, x.created_at DESC, x.id DESC
), sent AS (
    SELECT d.engagement_id, d.period,
           count(*) FILTER (WHERE l.status IN ('queued', 'claimed', 'delivered'))::bigint AS in_transfer,
           count(*) FILTER (WHERE l.status = 'failed')::bigint AS failed
    FROM latest l
    JOIN export_batch_items i ON i.engagement_id = l.engagement_id AND i.export_batch_id = l.export_batch_id
    JOIN journal_drafts d ON d.engagement_id = i.engagement_id AND d.id = i.journal_draft_id
    WHERE l.status IN ('queued', 'claimed', 'delivered', 'failed')
      AND i.organization_id = @organization_id AND i.reversed_at IS NULL
      AND d.organization_id = @organization_id
    GROUP BY d.engagement_id, d.period
), keys AS (
    SELECT engagement_id, period FROM approved
    UNION
    SELECT engagement_id, period FROM sent
)
SELECT v.uuid AS engagement_uuid, v.title AS taxpayer_title, k.period::text AS period,
       COALESCE(a.n, 0)::bigint AS approved,
       COALESCE(s.in_transfer, 0)::bigint AS in_transfer,
       COALESCE(s.failed, 0)::bigint AS failed,
       count(*) OVER ()::bigint AS total
FROM keys k
JOIN visible v ON v.id = k.engagement_id
LEFT JOIN approved a ON a.engagement_id = k.engagement_id AND a.period = k.period
LEFT JOIN sent s ON s.engagement_id = k.engagement_id AND s.period = k.period
ORDER BY k.period, v.title, v.id
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofis son aktarımlar; read_all yoksa engagement_assignments ile süzülür
-- name: ListOfficeAccountingTransfers :many
-- Ofis genelinde her çıktının son aktarımı (yeniden deneme eskisini gizler)
-- ve aktarılan dönemin en son mizan karşılaştırması (#574). check_issues:
-- tutmayan hesap + mizanda olmayan + kaynağı olmayan. Dönem seçmeden alınan
-- çıktının dönemi fişlerinden okunur (en eski).
SELECT x.uuid, x.status, x.driver, x.method, x.error_message, x.completed_at, x.created_at,
       btrim(COALESCE(cu.name, '') || ' ' || COALESCE(cu.surname, ''))::text AS completed_by_name,
       b.uuid AS batch_uuid, b.period AS batch_period,
       e.uuid AS engagement_uuid, t.title AS taxpayer_title,
       (r.id IS NOT NULL)::bool AS checked,
       r.created_at AS check_at,
       (COALESCE((r.summary->>'differences')::int, 0)
        + COALESCE((r.summary->>'missing_in_ledger')::int, 0)
        + COALESCE((r.summary->>'missing_in_sources')::int, 0))::bigint AS check_issues,
       count(*) OVER ()::bigint AS total
FROM (
    SELECT DISTINCT ON (a.export_batch_id) a.*
    FROM accounting_transfers a
    WHERE a.organization_id = @organization_id
      AND (@all_engagements::bool OR a.engagement_id IN (
            SELECT g.engagement_id FROM engagement_assignments g
            WHERE g.organization_id = @organization_id AND g.user_id = @user_id))
    ORDER BY a.export_batch_id, a.created_at DESC, a.id DESC
) x
JOIN (
    SELECT eb.id, eb.engagement_id, eb.uuid,
           COALESCE(eb.period, min(d.period))::text AS period
    FROM export_batches eb
    JOIN export_batch_items i ON i.engagement_id = eb.engagement_id AND i.export_batch_id = eb.id
    JOIN journal_drafts d ON d.engagement_id = i.engagement_id AND d.id = i.journal_draft_id
    WHERE eb.organization_id = @organization_id
    GROUP BY eb.id
) b ON b.id = x.export_batch_id AND b.engagement_id = x.engagement_id
JOIN engagements e ON e.organization_id = x.organization_id AND e.id = x.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
LEFT JOIN users cu ON cu.id = x.completed_by
LEFT JOIN LATERAL (
    SELECT rr.id, rr.created_at, rr.summary
    FROM reconciliation_runs rr
    WHERE rr.organization_id = x.organization_id AND rr.engagement_id = x.engagement_id
      AND rr.period = b.period
    ORDER BY rr.id DESC
    LIMIT 1
) r ON TRUE
WHERE (sqlc.narg(status)::text IS NULL OR x.status = sqlc.narg(status))
ORDER BY x.created_at DESC, x.id DESC
LIMIT @lim OFFSET @off;
