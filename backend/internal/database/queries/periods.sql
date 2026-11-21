-- Dönem yönetimi (issue #32): kilit, hazırlık durumu, değişiklik talepleri.

-- name: AcquirePeriodWriteLock :exec
-- Fatura yazımı: aynı mükellef + dönem kilitlenirken bekler (paylaşımlı, tx sonuna kadar).
SELECT pg_advisory_xact_lock_shared(hashtextextended(format('period_lock:%s:%s', @taxpayer_id::bigint, @period::text), 0));

-- name: AcquirePeriodLockExclusive :exec
-- Kilitleme: o mükellef + döneme süren fatura yazımları bitene kadar bekler.
SELECT pg_advisory_xact_lock(hashtextextended(format('period_lock:%s:%s', @taxpayer_id::bigint, @period::text), 0));

-- tenant-lint: cross-org fatura yazım yolu (kaynak veri, mükellef kapsamı): mükellefin aktif engagement'ındaki kilit (D1: tek aktif)
-- name: GetActivePeriodLockForTaxpayer :one
SELECT pl.organization_id, pl.engagement_id, pl.taxpayer_id, pl.period, pl.locked_at
FROM period_locks pl
JOIN engagements e ON e.organization_id = pl.organization_id AND e.id = pl.engagement_id
WHERE pl.taxpayer_id = @taxpayer_id AND pl.period = @period AND e.ended_at IS NULL
ORDER BY pl.id
LIMIT 1;

-- name: GetPeriodLock :one
SELECT * FROM period_locks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period;

-- name: InsertPeriodLock :one
INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period, note, locked_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, sqlc.narg(note), sqlc.narg(locked_by))
ON CONFLICT (engagement_id, period) DO NOTHING
RETURNING *;

-- name: DeletePeriodLock :execrows
DELETE FROM period_locks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND period = @period;

-- name: InsertPeriodLockEvent :one
INSERT INTO period_lock_events (organization_id, engagement_id, period, action, reason, actor_id)
VALUES (@organization_id, @engagement_id, @period, @action, sqlc.narg(reason), sqlc.narg(actor_id))
RETURNING *;

-- name: ListPeriodLockEvents :many
SELECT ev.uuid, ev.period, ev.action, ev.reason, u.uuid AS actor_uuid, ev.created_at
FROM period_lock_events ev
LEFT JOIN users u ON u.id = ev.actor_id
WHERE ev.organization_id = @organization_id AND ev.engagement_id = @engagement_id AND ev.period = @period
ORDER BY ev.created_at DESC, ev.id DESC;

-- name: CountOpenBlockersByCode :many
-- Hazırlık: dönemin açık engelleyici uyarıları. Dönemsiz fatura uyarıları
-- faturanın dönemine sayılır.
SELECT w.code, count(*)::bigint AS n
FROM data_warnings w
LEFT JOIN invoices inv ON w.entity_type = 'invoice' AND inv.taxpayer_id = w.taxpayer_id AND inv.id = w.entity_id
WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
  AND w.status = 'open' AND w.severity = 'blocker'
  AND COALESCE(w.period, inv.period) = @period::text
GROUP BY w.code
ORDER BY w.code;

-- name: CountOpenChangeRequestsForPeriod :one
SELECT count(*)::bigint
FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND period = @period AND status = 'open';

-- tenant-lint: org-wide ofis dönem matrisi; read_all yoksa engagement_assignments ile süzülür
-- name: ListPeriodMatrixEngagements :many
SELECT e.id, e.uuid, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title, t.tax_id AS taxpayer_tax_id, e.ledger_type
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
ORDER BY t.title, e.id
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofis dönem matrisi sayımı; ListPeriodMatrixEngagements ile aynı süzgeç
-- name: CountPeriodMatrixEngagements :one
SELECT count(*)::bigint
FROM engagements e
WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
  AND (@all_engagements::bool OR e.id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id));

-- name: PeriodMatrixBlockers :many
-- n: engelleyen açık uyarılar; open_warnings: seviyesi ne olursa olsun
-- açık uyarıların tamamı (#529: mükellef ve dönem bazında açık uyarı sayısı).
SELECT w.engagement_id, COALESCE(w.period, inv.period)::text AS period,
       (count(*) FILTER (WHERE w.severity = 'blocker'))::bigint AS n,
       count(*)::bigint AS open_warnings
FROM data_warnings w
LEFT JOIN invoices inv ON w.entity_type = 'invoice' AND inv.taxpayer_id = w.taxpayer_id AND inv.id = w.entity_id
WHERE w.organization_id = @organization_id AND w.engagement_id = ANY(@engagement_ids::bigint[])
  AND w.status = 'open'
  AND COALESCE(w.period, inv.period) = ANY(@periods::text[])
GROUP BY w.engagement_id, COALESCE(w.period, inv.period);

-- name: PeriodMatrixChangeRequests :many
SELECT engagement_id, period::text AS period, count(*)::bigint AS n
FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND status = 'open' AND period = ANY(@periods::text[])
GROUP BY engagement_id, period;

-- name: PeriodMatrixLocks :many
SELECT engagement_id, period::text AS period, locked_at
FROM period_locks
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND period = ANY(@periods::text[]);

-- name: PeriodMatrixReturns :many
-- Ofis beyan görünümü (#575): dönemin beyan kayıtları (kayıt yoksa hazırlanmadı).
SELECT engagement_id, period::text AS period, kind, status, payable_amount, filed_on, paid_on
FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND period = ANY(@periods::text[]);

-- name: PeriodMatrixDocumentTasks :many
-- Mükelleften beklenen (open) ve gelip işlenmeyen (submitted) belgeler.
SELECT engagement_id, period::text AS period,
       (count(*) FILTER (WHERE status = 'open'))::bigint AS open,
       (count(*) FILTER (WHERE status = 'submitted'))::bigint AS submitted
FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = ANY(@engagement_ids::bigint[])
  AND status IN ('open', 'submitted') AND period = ANY(@periods::text[])
GROUP BY engagement_id, period;

-- name: PeriodMatrixMissingStatements :many
-- Eksik banka ekstresi (#568): kapanmış bir ayda hiç hareketi olmayan, ama
-- önceki üç ayda hareketi olan banka hesabı. Kapanmış hesap üç ay sonra
-- düşer; açık ay (bugünün ayı) sayılmaz.
SELECT e.id AS engagement_id, p.period::text AS period, b.name::text AS name
FROM engagements e
JOIN cash_boxes b ON b.taxpayer_id = e.taxpayer_id AND b.type = 'bank_account'
CROSS JOIN unnest(@periods::text[]) AS p(period)
WHERE e.organization_id = @organization_id AND e.id = ANY(@engagement_ids::bigint[])
  AND p.period < to_char(now() AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')
  AND NOT EXISTS (
    SELECT 1 FROM bank_transactions t
    WHERE t.taxpayer_id = e.taxpayer_id AND t.cash_box_id = b.id AND t.period = p.period)
  AND EXISTS (
    SELECT 1 FROM bank_transactions t
    WHERE t.taxpayer_id = e.taxpayer_id AND t.cash_box_id = b.id AND t.period < p.period
      AND t.period >= to_char(to_date(p.period || '-01', 'YYYY-MM-DD') - interval '3 months', 'YYYY-MM'))
ORDER BY e.id, p.period, b.name;

-- name: PeriodMatrixPendingWork :many
-- Beyandan önce işlenmesi gereken kayıtlar (#575): eşleşmemiş, yok
-- sayılmamış ve bu angajmanda karşı hesabı onaylanmamış banka hareketleri;
-- onay bekleyen KDV'li masraf fişleri ve Z raporları.
SELECT e.id AS engagement_id, p.period::text AS period,
       (SELECT count(*) FROM bank_transactions b
        WHERE b.taxpayer_id = e.taxpayer_id AND b.period = p.period AND NOT b.is_cancelled
          AND b.match_status NOT IN ('matched', 'ignored')
          AND NOT EXISTS (SELECT 1 FROM bank_transaction_postings bp
                          WHERE bp.organization_id = e.organization_id AND bp.engagement_id = e.id
                            AND bp.bank_transaction_id = b.id AND bp.status = 'approved'))::bigint AS bank,
       (SELECT count(*) FROM expenses x
        WHERE x.taxpayer_id = e.taxpayer_id AND x.period = p.period
          AND COALESCE(x.tax_amount, 0) > 0 AND x.approval_status = 'pending')::bigint AS expenses,
       (SELECT count(*) FROM retail_sales r
        WHERE r.taxpayer_id = e.taxpayer_id AND r.period = p.period AND r.approval_status = 'pending')::bigint AS retail
FROM engagements e
CROSS JOIN unnest(@periods::text[]) AS p(period)
WHERE e.organization_id = @organization_id AND e.id = ANY(@engagement_ids::bigint[]);

-- name: FindOpenChangeRequestByHash :one
-- Aynı içerik ikinci kez gelirse açık talep tekrar kullanılır.
SELECT * FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND entity_type = @entity_type
  AND entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::bigint
  AND period = @period AND new_hash = @new_hash AND status = 'open'
ORDER BY id
LIMIT 1;

-- name: CountOpenChangeRequestsForEntity :one
SELECT count(*)::bigint
FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND entity_type = @entity_type
  AND entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::bigint
  AND period = @period AND status = 'open';

-- name: ListChangeRequests :many
SELECT cr.id, cr.uuid, cr.period, cr.entity_type, cr.entity_id, inv.uuid AS invoice_uuid,
       cr.old_hash, cr.new_hash, cr.diff, cr.status, u.uuid AS decided_by_uuid, cr.decided_at,
       cr.created_at, cr.updated_at
FROM change_requests cr
LEFT JOIN invoices inv ON cr.entity_type = 'invoice' AND inv.taxpayer_id = cr.taxpayer_id AND inv.id = cr.entity_id
LEFT JOIN users u ON u.id = cr.decided_by
WHERE cr.organization_id = @organization_id AND cr.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR cr.status = sqlc.narg(status)::text)
  AND (sqlc.narg(period)::text IS NULL OR cr.period = sqlc.narg(period)::text)
ORDER BY cr.created_at DESC, cr.id DESC
LIMIT @lim OFFSET @off;

-- name: CountChangeRequests :one
SELECT count(*)::bigint
FROM change_requests cr
WHERE cr.organization_id = @organization_id AND cr.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR cr.status = sqlc.narg(status)::text)
  AND (sqlc.narg(period)::text IS NULL OR cr.period = sqlc.narg(period)::text);

-- name: GetChangeRequest :one
SELECT cr.id, cr.uuid, cr.period, cr.entity_type, cr.entity_id, inv.uuid AS invoice_uuid,
       cr.old_hash, cr.new_hash, cr.diff, cr.new_payload, cr.status, u.uuid AS decided_by_uuid, cr.decided_at,
       cr.created_at, cr.updated_at
FROM change_requests cr
LEFT JOIN invoices inv ON cr.entity_type = 'invoice' AND inv.taxpayer_id = cr.taxpayer_id AND inv.id = cr.entity_id
LEFT JOIN users u ON u.id = cr.decided_by
WHERE cr.organization_id = @organization_id AND cr.engagement_id = @engagement_id AND cr.uuid = @uuid;

-- name: LockChangeRequest :one
-- Karar: talebi satır kilidiyle okur (eşzamanlı iki karar tek sonuç verir).
SELECT * FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
FOR UPDATE;
