-- Eksik belge görevi (issue #36). Banka tabloları yalnızca okunur.

-- name: ListMissingDocumentCandidates :many
-- Kontrolün baktığı hareketler: dönemin tedarikçi ödemesi / müşteri
-- tahsilatı hareketleri ve bu üreticinin hâlâ açık uyarısı olan hareketler
-- (sınıflandırma değiştiyse uyarı kapanabilsin diye).
SELECT bt.id, bt.uuid, bt.period, bt.debit_credit, bt.kind_detail, bt.match_status, bt.is_cancelled,
       bt.amount, bt.currency, bt.transaction_date, bt.counterparty_name,
       EXISTS (
           SELECT 1 FROM bank_transaction_documents d
           WHERE d.taxpayer_id = bt.taxpayer_id AND d.bank_transaction_id = bt.id
       ) AS has_document
FROM bank_transactions bt
WHERE bt.taxpayer_id = @taxpayer_id
  AND bt.period = @period
  AND (
      bt.kind_detail IN ('supplier_payment', 'customer_collection')
      OR EXISTS (
          SELECT 1 FROM data_warnings w
          WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
            AND w.entity_type = 'bank_transaction' AND w.entity_id = bt.id
            AND w.producer = @producer AND w.status = 'open'
      )
  )
ORDER BY bt.transaction_date, bt.id;

-- name: GetMissingDocumentCandidate :one
SELECT bt.id, bt.uuid, bt.period, bt.debit_credit, bt.kind_detail, bt.match_status, bt.is_cancelled,
       bt.amount, bt.currency, bt.transaction_date, bt.counterparty_name,
       EXISTS (
           SELECT 1 FROM bank_transaction_documents d
           WHERE d.taxpayer_id = bt.taxpayer_id AND d.bank_transaction_id = bt.id
       ) AS has_document
FROM bank_transactions bt
WHERE bt.taxpayer_id = @taxpayer_id AND bt.id = @id;

-- name: CreateDocumentTasksFromWarnings :many
-- Açık BNK_* uyarılarından görev açar. Hareket başına tek açık görev
-- (uq_document_tasks_active); var olanlar atlanır.
INSERT INTO document_tasks (
    organization_id, engagement_id, taxpayer_id, bank_transaction_id, warning_id,
    code, kind, period, note, due_at, next_reminder_at, created_by
)
SELECT w.organization_id, w.engagement_id, w.taxpayer_id, w.entity_id, w.id,
       w.code,
       CASE w.code WHEN 'BNK_UNDOCUMENTED_PAYMENT' THEN 'undocumented_payment' ELSE 'uninvoiced_collection' END,
       w.period, @note, @due_at, @next_reminder_at, sqlc.narg(created_by)
FROM data_warnings w
WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
  AND w.status = 'open'
  AND w.entity_type = 'bank_transaction' AND w.entity_id IS NOT NULL AND w.period IS NOT NULL
  AND w.code IN ('BNK_UNDOCUMENTED_PAYMENT', 'BNK_UNINVOICED_COLLECTION')
  AND (sqlc.narg(period)::text IS NULL OR w.period = sqlc.narg(period))
  AND (sqlc.narg(warning_uuids)::uuid[] IS NULL OR w.uuid = ANY (sqlc.narg(warning_uuids)::uuid[]))
ORDER BY w.id
ON CONFLICT (engagement_id, bank_transaction_id) WHERE status IN ('open', 'submitted') DO NOTHING
RETURNING id;

-- name: ListDocumentTasks :many
SELECT t.id, t.uuid, t.organization_id, t.engagement_id, t.taxpayer_id, t.bank_transaction_id,
       t.code, t.kind, t.period, t.status, t.title, t.request_count, t.note, t.due_at, t.reminder_count, t.last_reminded_at,
       t.next_reminder_at, t.submitted_at, t.matched_at, t.cancelled_at, t.cancel_reason,
       t.created_at, t.updated_at, t.channels, t.remind_days_before, t.channel_remind_at, t.channel_reminded_at,
       bt.uuid AS bank_transaction_uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.counterparty_name, bt.description,
       (SELECT count(*) FROM document_task_uploads u
        WHERE u.organization_id = t.organization_id AND u.engagement_id = t.engagement_id AND u.task_id = t.id)::bigint AS upload_count
FROM document_tasks t
LEFT JOIN bank_transactions bt ON bt.taxpayer_id = t.taxpayer_id AND bt.id = t.bank_transaction_id
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period))
ORDER BY t.created_at DESC, t.id DESC
LIMIT @lim OFFSET @off;

-- name: CountDocumentTasks :one
SELECT count(*) FROM document_tasks t
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period));

-- name: GetDocumentTask :one
SELECT t.id, t.uuid, t.organization_id, t.engagement_id, t.taxpayer_id, t.bank_transaction_id,
       t.code, t.kind, t.period, t.status, t.title, t.request_count, t.note, t.due_at, t.reminder_count, t.last_reminded_at,
       t.next_reminder_at, t.submitted_at, t.matched_at, t.cancelled_at, t.cancel_reason,
       t.created_at, t.updated_at, t.channels, t.remind_days_before, t.channel_remind_at, t.channel_reminded_at,
       bt.uuid AS bank_transaction_uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.counterparty_name, bt.description,
       (SELECT count(*) FROM document_task_uploads u
        WHERE u.organization_id = t.organization_id AND u.engagement_id = t.engagement_id AND u.task_id = t.id)::bigint AS upload_count
FROM document_tasks t
LEFT JOIN bank_transactions bt ON bt.taxpayer_id = t.taxpayer_id AND bt.id = t.bank_transaction_id
WHERE t.organization_id = @organization_id AND t.engagement_id = @engagement_id AND t.uuid = @uuid;

-- name: CancelDocumentTask :execrows
UPDATE document_tasks
SET status = 'cancelled', cancelled_at = NOW(), cancelled_by = sqlc.narg(cancelled_by),
    cancel_reason = @cancel_reason, next_reminder_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status IN ('open', 'submitted');

-- name: CloseResolvedDocumentTasks :execrows
-- Koşul başka yolla kalktı (ör. müşavir faturayla eşledi): açık görev kapanır.
UPDATE document_tasks
SET status = 'cancelled', cancelled_at = NOW(), cancel_reason = 'resolved', next_reminder_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND bank_transaction_id = ANY (@bank_transaction_ids::bigint[])
  AND status = 'open';

-- name: ListDocumentTaskUploads :many
SELECT u.uuid, u.created_at, a.uuid AS attachment_uuid, a.kind, a.content_type, a.file_name, a.size_bytes, a.sha256
FROM document_task_uploads u
JOIN attachments a ON a.taxpayer_id = u.taxpayer_id AND a.id = u.attachment_id
WHERE u.organization_id = @organization_id AND u.engagement_id = @engagement_id AND u.task_id = @task_id
ORDER BY u.created_at, u.id;

-- name: GetDocumentTaskUploadFile :one
SELECT a.storage_key, a.content_type, a.file_name, a.size_bytes
FROM document_task_uploads u
JOIN document_tasks t ON t.organization_id = u.organization_id AND t.id = u.task_id
JOIN attachments a ON a.taxpayer_id = u.taxpayer_id AND a.id = u.attachment_id
WHERE u.organization_id = @organization_id AND u.engagement_id = @engagement_id
  AND t.uuid = @task_uuid AND u.uuid = @upload_uuid;

-- ---------------------------------------------------------------------------
-- Mükellef tarafı (/v1/taxpayer/*). Kapsam X-Taxpayer üyeliğidir: görevler
-- taxpayer_id ile ve yalnızca mükellefin AKTİF engagement'ından okunur.
-- Bitmiş engagement'ın (eski ofisin) görevleri görünmez (D1, D2).
-- ---------------------------------------------------------------------------

-- tenant-lint: cross-org mükellef kendi görevlerini görür; taxpayer_id ve aktif engagement ile sınırlı
-- name: ListTaxpayerDocumentTasks :many
SELECT t.id, t.uuid, t.organization_id, t.engagement_id, t.taxpayer_id, t.bank_transaction_id,
       t.code, t.kind, t.period, t.status, t.title, t.note, t.due_at, t.submitted_at, t.matched_at,
       t.created_at, t.updated_at,
       o.name AS organization_name,
       bt.uuid AS bank_transaction_uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.counterparty_name, bt.description,
       (SELECT count(*) FROM document_task_uploads u
        WHERE u.organization_id = t.organization_id AND u.engagement_id = t.engagement_id AND u.task_id = t.id)::bigint AS upload_count
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
JOIN organizations o ON o.id = t.organization_id
LEFT JOIN bank_transactions bt ON bt.taxpayer_id = t.taxpayer_id AND bt.id = t.bank_transaction_id
WHERE t.taxpayer_id = @taxpayer_id AND e.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period))
ORDER BY t.due_at, t.id
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org mükellef kendi görevlerini sayar; taxpayer_id ve aktif engagement ile sınırlı
-- name: CountTaxpayerDocumentTasks :one
SELECT count(*)
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
WHERE t.taxpayer_id = @taxpayer_id AND e.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
  AND (sqlc.narg(period)::text IS NULL OR t.period = sqlc.narg(period));

-- tenant-lint: cross-org mükellef kendi görevini okur; taxpayer_id ve aktif engagement ile sınırlı
-- name: GetTaxpayerDocumentTask :one
SELECT t.id, t.uuid, t.organization_id, t.engagement_id, t.taxpayer_id, t.bank_transaction_id,
       t.code, t.kind, t.period, t.status, t.title, t.note, t.due_at, t.submitted_at, t.matched_at,
       t.created_at, t.updated_at,
       o.name AS organization_name,
       bt.uuid AS bank_transaction_uuid, bt.transaction_date, bt.debit_credit, bt.amount, bt.currency,
       bt.counterparty_name, bt.description,
       (SELECT count(*) FROM document_task_uploads u
        WHERE u.organization_id = t.organization_id AND u.engagement_id = t.engagement_id AND u.task_id = t.id)::bigint AS upload_count
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
JOIN organizations o ON o.id = t.organization_id
LEFT JOIN bank_transactions bt ON bt.taxpayer_id = t.taxpayer_id AND bt.id = t.bank_transaction_id
WHERE t.taxpayer_id = @taxpayer_id AND e.taxpayer_id = @taxpayer_id AND t.uuid = @uuid;

-- tenant-lint: cross-org mükellef yüklemesi için görevi kilitler; taxpayer_id ve aktif engagement ile sınırlı
-- name: LockTaxpayerDocumentTask :one
SELECT t.id, t.organization_id, t.engagement_id, t.taxpayer_id, t.bank_transaction_id, t.status, t.kind, t.title, t.created_by
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
WHERE t.taxpayer_id = @taxpayer_id AND e.taxpayer_id = @taxpayer_id AND t.uuid = @uuid
FOR UPDATE OF t;

-- name: InsertDocumentTaskUpload :one
INSERT INTO document_task_uploads (organization_id, engagement_id, taxpayer_id, task_id, attachment_id, uploaded_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @task_id, @attachment_id, sqlc.narg(uploaded_by))
ON CONFLICT (task_id, attachment_id) DO UPDATE SET uploaded_by = EXCLUDED.uploaded_by
RETURNING *;

-- name: InsertBankTransactionDocument :exec
INSERT INTO bank_transaction_documents (taxpayer_id, bank_transaction_id, attachment_id, method, matched_by)
VALUES (@taxpayer_id, @bank_transaction_id, @attachment_id, @method, sqlc.narg(matched_by))
ON CONFLICT (taxpayer_id, bank_transaction_id, attachment_id) DO NOTHING;

-- name: IsBankTransactionCancelled :one
SELECT is_cancelled FROM bank_transactions WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: SetDocumentTaskStatus :exec
-- matched: belge eşlendi; submitted: yüklendi, ofis bakacak. Hatırlatma durur.
UPDATE document_tasks
SET status = @status,
    submitted_at = COALESCE(submitted_at, NOW()),
    matched_at = CASE WHEN @status = 'matched' THEN NOW() ELSE matched_at END,
    next_reminder_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
  AND status IN ('open', 'submitted');

-- tenant-lint: cross-org mükellef tarafı yükleme indirme; taxpayer_id ve aktif engagement ile sınırlı
-- name: GetTaxpayerDocumentTaskUploadFile :one
SELECT a.storage_key, a.content_type, a.file_name, a.size_bytes
FROM document_task_uploads u
JOIN document_tasks t ON t.organization_id = u.organization_id AND t.id = u.task_id
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
JOIN attachments a ON a.taxpayer_id = u.taxpayer_id AND a.id = u.attachment_id
WHERE t.taxpayer_id = @taxpayer_id AND u.taxpayer_id = @taxpayer_id
  AND t.uuid = @task_uuid AND u.uuid = @upload_uuid;

-- ---------------------------------------------------------------------------
-- Hatırlatma (zamanlanmış iş).
-- ---------------------------------------------------------------------------

-- tenant-lint: cross-org zamanlanmış hatırlatma işi bütün ofislerin vadesi gelen görevlerini tarar
-- name: ListDueDocumentTaskReminders :many
-- Hatırlatması gelen açık görevler, engagement başına toplanır.
SELECT t.organization_id, t.engagement_id, t.taxpayer_id,
       count(*)::bigint AS task_count,
       count(*) FILTER (WHERE t.due_at <= @now)::bigint AS overdue_count,
       array_agg(t.id ORDER BY t.id)::bigint[] AS task_ids
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
WHERE t.status = 'open' AND t.next_reminder_at IS NOT NULL AND t.next_reminder_at <= @now
GROUP BY t.organization_id, t.engagement_id, t.taxpayer_id
ORDER BY t.organization_id, t.engagement_id
LIMIT @lim;

-- name: MarkDocumentTasksReminded :execrows
-- Geri çekilme: n. hatırlatmadan sonra bir sonraki base_days * 2^n gün sonra;
-- max_reminders'a ulaşınca hatırlatma biter (next_reminder_at NULL).
UPDATE document_tasks
SET reminder_count = reminder_count + 1,
    last_reminded_at = @now,
    next_reminder_at = CASE
        WHEN reminder_count + 1 >= @max_reminders::int THEN NULL
        ELSE sqlc.arg(now)::timestamptz + make_interval(days => sqlc.arg(base_days)::int * power(2, LEAST(reminder_count + 1, 8))::int)
    END
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND id = ANY (@ids::bigint[]) AND status = 'open';

-- name: ListDocumentTaskRecipients :many
SELECT user_id FROM taxpayer_members WHERE taxpayer_id = @taxpayer_id ORDER BY user_id;

-- name: GetDocumentTaskNotifyNames :one
SELECT o.name AS organization_name, o.slug AS organization_slug, tp.title AS taxpayer_title, tp.uuid AS taxpayer_uuid
FROM organizations o, taxpayers tp
WHERE o.id = @organization_id AND tp.id = @taxpayer_id;

-- name: GetDocumentTaskUUID :one
-- Tek görevli bildirimin bağlantısı görevin kendisine gider (#201).
SELECT uuid FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- ---------------------------------------------------------------------------
-- Müşavirin elle açtığı belge talebi (issue #240).
-- ---------------------------------------------------------------------------

-- name: CreateDocumentRequest :one
INSERT INTO document_tasks (
    organization_id, engagement_id, taxpayer_id, bank_transaction_id, code, kind, period,
    title, note, due_at, next_reminder_at, created_by, channels, remind_days_before, channel_remind_at
)
VALUES (@organization_id, @engagement_id, @taxpayer_id, NULL, 'DOC_REQUEST', 'document_request', @period,
        @title, @note, @due_at, sqlc.narg(next_reminder_at), sqlc.narg(created_by), @channels::text[],
        sqlc.narg(remind_days_before), sqlc.narg(channel_remind_at))
RETURNING id, uuid;

-- name: OpenDocumentRequestByTitle :one
-- Aynı mükellefe aynı başlıkla açık bir talep varsa ikincisi açılmaz (#568).
SELECT uuid FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND kind = 'document_request' AND status IN ('open', 'submitted')
  AND lower(title) = lower(@title::text)
ORDER BY id DESC
LIMIT 1;

-- name: CompleteDocumentTask :execrows
-- Ofis yüklenen belgeyi kabul etti: görev tamamlanır (matched).
UPDATE document_tasks
SET status = 'matched', matched_at = NOW(), next_reminder_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status IN ('open', 'submitted');

-- name: CloseDocumentRequestByTitle :execrows
-- İstenen belge başka yoldan geldi (ekstre içe aktarıldı, o ayın hareketleri
-- var): aynı başlıklı açık ya da gönderilmiş istek tamamlanır, mükellefe
-- hatırlatma gitmez.
UPDATE document_tasks
SET status = 'matched', matched_at = NOW(), next_reminder_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND kind = 'document_request' AND status IN ('open', 'submitted')
  AND lower(title) = lower(@title::text);

-- name: RerequestDocumentTask :execrows
-- Yüklenen belge yeterli değil: görev yeniden açılır, son tarih ve
-- hatırlatma baştan kurulur.
UPDATE document_tasks
SET status = 'open', note = @note, due_at = @due_at, next_reminder_at = sqlc.narg(next_reminder_at),
    reminder_count = 0, last_reminded_at = NULL, request_count = request_count + 1,
    channel_remind_at = sqlc.narg(channel_remind_at), channel_reminded_at = NULL
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
  AND status = 'submitted';

-- name: ListDocumentTaskOfficeRecipients :many
-- Yüklemeyi duyacak ofis üyeleri: görevi açan ve engagement'a atananlar
-- (hâlâ ofis üyesi olanlar).
SELECT DISTINCT m.user_id
FROM organization_members m
WHERE m.organization_id = @organization_id
  AND (m.user_id = sqlc.narg(created_by)::bigint
       OR m.user_id IN (SELECT a.user_id FROM engagement_assignments a
                        WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id))
ORDER BY m.user_id;

-- name: GetEngagementUUIDByID :one
SELECT uuid FROM engagements WHERE organization_id = @organization_id AND id = @id;

-- Belge talebi kanalları (Oğuz'un geri bildirimi): seçili kanallardan giden
-- hatırlatma. Elle "Hatırlat" ve zamanlanmış hatırlatma aynı sayaçları
-- işler.

-- name: MarkDocumentTaskChannelReminded :execrows
-- Elle hatırlatma: zamanlanmış hatırlatma (channel_remind_at) yerinde kalır.
UPDATE document_tasks
SET reminder_count = reminder_count + 1, last_reminded_at = @now, channel_reminded_at = @now
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id AND status = 'open';

-- tenant-lint: cross-org zamanlanmış hatırlatma işi bütün ofislerin vadesi gelen kanallı taleplerini tarar
-- name: ListDueChannelReminders :many
SELECT t.id, t.uuid, t.organization_id, t.engagement_id, t.taxpayer_id, e.uuid AS engagement_uuid
FROM document_tasks t
JOIN engagements e ON e.organization_id = t.organization_id AND e.id = t.engagement_id AND e.ended_at IS NULL
WHERE t.status = 'open' AND t.channel_remind_at IS NOT NULL AND t.channel_remind_at <= @now
ORDER BY t.channel_remind_at, t.id
LIMIT @lim;

-- name: FinishScheduledChannelReminder :execrows
-- Zamanlanmış hatırlatma gitti (ya da ofis ayarı kapalı olduğu için
-- gönderilmedi): bir daha denenmez.
UPDATE document_tasks
SET channel_remind_at = NULL,
    reminder_count = reminder_count + CASE WHEN @sent::bool THEN 1 ELSE 0 END,
    last_reminded_at = CASE WHEN @sent::bool THEN sqlc.arg(now)::timestamptz ELSE last_reminded_at END,
    channel_reminded_at = CASE WHEN @sent::bool THEN sqlc.arg(now)::timestamptz ELSE channel_reminded_at END
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;
