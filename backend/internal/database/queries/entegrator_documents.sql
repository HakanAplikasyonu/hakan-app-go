-- name: UpsertEntegratorDocument :one
-- Listeden görülen belge (37 §5.2). prev_* alanları bu çağrıdan önceki hali
-- verir: durum değiştiyse fatura güncellenir, UBL yoksa kuyruğa girer.
WITH prev AS (
    SELECT d.status_raw FROM entegrator_documents d
    WHERE d.taxpayer_id = @taxpayer_id AND d.connection_id = @connection_id AND d.ettn = @ettn AND d.direction = @direction
)
INSERT INTO entegrator_documents (
    taxpayer_id, connection_id, ettn, doc_kind, direction, invoice_number, issue_date, created_at_source,
    status_raw, status_normalized, warnings, header_raw)
VALUES (
    @taxpayer_id, @connection_id, @ettn, @doc_kind, @direction, sqlc.narg(invoice_number), sqlc.narg(issue_date), sqlc.narg(created_at_source),
    @status_raw, @status_normalized, @warnings::text[], @header_raw)
ON CONFLICT (connection_id, ettn, direction) DO UPDATE SET
    doc_kind = EXCLUDED.doc_kind,
    invoice_number = COALESCE(EXCLUDED.invoice_number, entegrator_documents.invoice_number),
    issue_date = COALESCE(EXCLUDED.issue_date, entegrator_documents.issue_date),
    created_at_source = COALESCE(EXCLUDED.created_at_source, entegrator_documents.created_at_source),
    status_raw = EXCLUDED.status_raw,
    status_normalized = EXCLUDED.status_normalized,
    warnings = EXCLUDED.warnings,
    header_raw = EXCLUDED.header_raw,
    last_seen_at = NOW(),
    missing_since = NULL
RETURNING id, ubl_status, invoice_id,
    EXISTS (SELECT 1 FROM prev)::bool AS existed,
    COALESCE((SELECT status_raw FROM prev), '')::text AS prev_status_raw;

-- name: GetEntegratorDocument :one
SELECT * FROM entegrator_documents
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND ettn = @ettn AND direction = @direction;

-- name: ListEntegratorDocuments :many
SELECT * FROM entegrator_documents
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id
ORDER BY created_at_source NULLS LAST, id;

-- name: ListEntegratorUBLQueue :many
-- UBL kuyruğu (37 §5.4): bekleyenler ve deneme hakkı kalan hatalılar.
SELECT * FROM entegrator_documents
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id
  AND id > @after_id
  AND (ubl_status = 'pending' OR (ubl_status = 'failed' AND ubl_attempts < @max_attempts::int))
ORDER BY id
LIMIT @lim;

-- name: SetEntegratorDocumentUBL :execrows
UPDATE entegrator_documents
SET ubl_status = @ubl_status,
    ubl_attempts = ubl_attempts + CASE WHEN @ubl_status::text = 'failed' THEN 1 ELSE 0 END,
    ubl_error = sqlc.narg(ubl_error),
    ubl_attachment_id = COALESCE(sqlc.narg(ubl_attachment_id), ubl_attachment_id),
    invoice_id = COALESCE(sqlc.narg(invoice_id), invoice_id)
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: MarkEntegratorDocumentsMissing :execrows
-- Tam taramada görülmeyen belgeler (37 §5.5). Silinmez; ilk görülmediği an kalır.
UPDATE entegrator_documents
SET missing_since = COALESCE(missing_since, NOW())
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id
  AND direction = @direction AND doc_kind = @doc_kind
  AND created_at_source >= @range_start AND created_at_source < @range_end
  AND last_seen_at < @seen_before;

-- name: SetInvoiceStatusNormalized :execrows
-- Entegratör durumu (30 §8) faturaya işlenir.
UPDATE invoices SET status_normalized = @status_normalized
WHERE taxpayer_id = @taxpayer_id AND id = @id AND status_normalized <> @status_normalized;

-- name: SetSourceConnectionCapabilities :execrows
UPDATE source_connections SET capabilities = @capabilities
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: LastSyncRunStartedAt :one
SELECT MAX(started_at)::timestamptz AS started_at FROM sync_runs
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND kind = @kind;

-- name: SetSyncWindowCursor :execrows
-- Sayfa sonrası ilerleme; yarıda kalan pencere buradan sürer (deneme sayısı artmaz).
UPDATE sync_windows SET page_cursor = sqlc.narg(page_cursor)
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: ListResumableSyncWindows :many
-- Bağlantı kilidi alındıktan sonra çağrılır: 'running' kalan pencere yarıda
-- kesilmiş bir çalışmadandır ve imlecinden sürer. Yeniden eskiye: bu ayın
-- faturaları önce gelir (#780).
SELECT * FROM sync_windows
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id
  AND status IN ('pending', 'failed', 'running') AND attempts < @max_attempts::int
ORDER BY range_start DESC, id;

-- name: PlanSyncWindow :one
-- Planlanan pencere: aynı aralık daha önce bittiyse yeniden listelenmek
-- üzere 'pending'e döner (tam tarama, yeniden ilk yükleme). Çalışan ya da
-- hatalı pencere olduğu gibi kalır.
INSERT INTO sync_windows (taxpayer_id, connection_id, item, direction, date_field, range_start, range_end)
VALUES (@taxpayer_id, @connection_id, @item, sqlc.narg(direction), @date_field, @range_start, @range_end)
ON CONFLICT (connection_id, item, date_field, range_start, range_end) DO UPDATE SET
    status = CASE WHEN sync_windows.status = 'done' THEN 'pending' ELSE sync_windows.status END,
    page_cursor = CASE WHEN sync_windows.status = 'done' THEN NULL ELSE sync_windows.page_cursor END,
    attempts = CASE WHEN sync_windows.status = 'done' THEN 0 ELSE sync_windows.attempts END,
    last_error = CASE WHEN sync_windows.status = 'done' THEN NULL ELSE sync_windows.last_error END,
    updated_at = NOW()
RETURNING *;

-- name: SyncWindowMonths :many
-- İlk yükleme ilerlemesi (#780): since'ten bu yana her takvim ayının
-- (İstanbul) pencere sayısı ve bitenleri. Hatalı pencere de bitmiş sayılır
-- (sonraki çalıştırmalar yeniden dener).
SELECT to_char(range_start AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM')::text AS month,
       COUNT(*)::int AS total,
       COUNT(*) FILTER (WHERE status NOT IN ('pending', 'running'))::int AS settled
FROM sync_windows
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND range_start >= @since
GROUP BY 1
ORDER BY 1;

-- name: EntegratorDocumentProgress :one
-- İlk yükleme ilerlemesi (#780): since'ten bu yana oluşturulan belgeler,
-- bu ayınkiler ve UBL'i (fatura) bekleyenler.
SELECT COUNT(*) FILTER (WHERE created_at_source >= @month_start)::int AS this_month,
       COUNT(*) FILTER (WHERE created_at_source >= @month_start AND invoice_id IS NOT NULL)::int AS this_month_invoices,
       COUNT(*) FILTER (WHERE ubl_status = 'pending' OR (ubl_status = 'failed' AND ubl_attempts < @max_attempts::int))::int AS ubl_pending
FROM entegrator_documents
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND created_at_source >= @since;
