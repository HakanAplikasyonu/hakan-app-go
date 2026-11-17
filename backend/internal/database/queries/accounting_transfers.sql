-- Muhasebe programına aktarım görevleri (issue #391).

-- name: InsertAccountingTransfer :one
-- Satır dönmezse çıktının açık ya da aktarılmış görevi var.
INSERT INTO accounting_transfers (organization_id, engagement_id, export_batch_id, driver, method, requested_by)
VALUES (@organization_id, @engagement_id, @export_batch_id, @driver, @method, @requested_by)
ON CONFLICT (export_batch_id) WHERE status IN ('queued', 'claimed', 'delivered', 'imported') DO NOTHING
RETURNING *;

-- name: GetAccountingTransfer :one
SELECT t.*, b.uuid AS batch_uuid, b.period AS batch_period, b.files AS batch_files,
       a.uuid AS agent_uuid, a.name AS agent_name
FROM accounting_transfers t
JOIN export_batches b ON b.id = t.export_batch_id AND b.engagement_id = t.engagement_id
LEFT JOIN desktop_agents a ON a.id = t.agent_id AND a.organization_id = t.organization_id
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id AND t.uuid = @uuid;

-- name: ListAccountingTransfers :many
SELECT t.*, b.uuid AS batch_uuid, b.period AS batch_period, b.files AS batch_files,
       a.uuid AS agent_uuid, a.name AS agent_name
FROM accounting_transfers t
JOIN export_batches b ON b.id = t.export_batch_id AND b.engagement_id = t.engagement_id
LEFT JOIN desktop_agents a ON a.id = t.agent_id AND a.organization_id = t.organization_id
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
