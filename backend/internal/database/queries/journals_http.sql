-- Yevmiye ve çıktı uçları (issue #143): ofis panelindeki listeler.

-- name: ListJournalDrafts :many
SELECT d.*,
       COALESCE(t.debit, 0)::numeric AS total_debit,
       COALESCE(t.credit, 0)::numeric AS total_credit,
       COALESCE(t.lines, 0)::int AS line_count
FROM journal_drafts d
LEFT JOIN LATERAL (
    SELECT SUM(l.debit) AS debit, SUM(l.credit) AS credit, COUNT(*) AS lines
    FROM journal_draft_lines l
    WHERE l.organization_id = d.organization_id AND l.engagement_id = d.engagement_id AND l.journal_draft_id = d.id
) t ON TRUE
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR d.period = sqlc.narg(period))
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
ORDER BY d.period DESC, d.entry_no DESC
LIMIT @lim OFFSET @off;

-- name: CountJournalDrafts :one
SELECT COUNT(*)::bigint FROM journal_drafts d
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR d.period = sqlc.narg(period))
  AND (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status));

-- name: ListExportBatches :many
SELECT b.*,
       (SELECT COUNT(*) FROM export_batch_items i
         WHERE i.organization_id = b.organization_id AND i.engagement_id = b.engagement_id
           AND i.export_batch_id = b.id)::int AS entry_count,
       (SELECT COUNT(*) FROM export_batch_items i
         WHERE i.organization_id = b.organization_id AND i.engagement_id = b.engagement_id
           AND i.export_batch_id = b.id AND i.reversed_at IS NOT NULL)::int AS reversed_count
FROM export_batches b
WHERE b.organization_id = @organization_id AND b.engagement_id = @engagement_id
ORDER BY b.created_at DESC, b.id DESC
LIMIT @lim OFFSET @off;
