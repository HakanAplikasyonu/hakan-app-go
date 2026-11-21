-- İletişim merkezi (#780 C): otomasyon kuralları, çalışmaları ve geçmişi.

-- name: GetOfficeAutomationSettings :one
SELECT * FROM office_automation_settings WHERE organization_id = @organization_id;

-- name: UpsertOfficeAutomationSettings :one
INSERT INTO office_automation_settings (organization_id, enabled, updated_by, updated_at)
VALUES (@organization_id, @enabled, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;

-- name: InsertAutomationTemplate :execrows
-- Hazır kural: ofiste yoksa pasif kurulur; silinmiş ya da düzenlenmiş olana
-- dokunulmaz.
INSERT INTO automation_rules (organization_id, template_code, name, trigger_kind, trigger_config, target_kind,
    only_due, channels, sms_if_unopened_hours, subject, body, approval_mode, active, active_since)
VALUES (@organization_id, @template_code, @name, @trigger_kind, @trigger_config, 'all',
    @only_due, @channels::text[], sqlc.narg(sms_if_unopened_hours), @subject, @body, @approval_mode, FALSE, NULL)
ON CONFLICT (organization_id, template_code) WHERE template_code <> '' DO NOTHING;

-- name: ListAutomationRules :many
SELECT * FROM automation_rules
WHERE organization_id = @organization_id AND deleted_at IS NULL
ORDER BY (template_code = '') DESC, created_at DESC, id DESC;

-- name: GetAutomationRule :one
SELECT * FROM automation_rules
WHERE organization_id = @organization_id AND uuid = @uuid AND deleted_at IS NULL;

-- name: GetAutomationRuleByID :one
SELECT * FROM automation_rules
WHERE organization_id = @organization_id AND id = @id;

-- name: InsertAutomationRule :one
INSERT INTO automation_rules (organization_id, name, trigger_kind, trigger_config, target_kind, target_group_uuid,
    target_engagements, only_due, channels, sms_if_unopened_hours, subject, body, approval_mode, active, active_since,
    created_by, updated_by)
VALUES (@organization_id, @name, @trigger_kind, @trigger_config, @target_kind, sqlc.narg(target_group_uuid),
    @target_engagements::uuid[], @only_due, @channels::text[], sqlc.narg(sms_if_unopened_hours), @subject, @body,
    @approval_mode, @active, CASE WHEN @active::boolean THEN NOW() ELSE NULL END, sqlc.narg(actor), sqlc.narg(actor))
RETURNING *;

-- name: UpdateAutomationRule :one
UPDATE automation_rules
SET name = @name, trigger_kind = @trigger_kind, trigger_config = @trigger_config, target_kind = @target_kind,
    target_group_uuid = sqlc.narg(target_group_uuid), target_engagements = @target_engagements::uuid[],
    only_due = @only_due, channels = @channels::text[], sms_if_unopened_hours = sqlc.narg(sms_if_unopened_hours),
    subject = @subject, body = @body, approval_mode = @approval_mode,
    active_since = CASE WHEN @active::boolean AND NOT active THEN NOW() WHEN NOT @active::boolean THEN NULL ELSE active_since END,
    active = @active::boolean, updated_by = sqlc.narg(actor)
WHERE organization_id = @organization_id AND uuid = @uuid AND deleted_at IS NULL
RETURNING *;

-- name: SetAutomationRuleActive :one
UPDATE automation_rules
SET active_since = CASE WHEN @active::boolean AND NOT active THEN NOW() WHEN NOT @active::boolean THEN NULL ELSE active_since END,
    active = @active::boolean, updated_by = sqlc.narg(actor)
WHERE organization_id = @organization_id AND uuid = @uuid AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteAutomationRule :one
UPDATE automation_rules
SET deleted_at = NOW(), active = FALSE, active_since = NULL, updated_by = sqlc.narg(actor)
WHERE organization_id = @organization_id AND uuid = @uuid AND deleted_at IS NULL
RETURNING *;

-- name: TouchAutomationRuleSent :exec
UPDATE automation_rules
SET send_count = send_count + 1, last_run_at = NOW()
WHERE organization_id = @organization_id AND id = @id;

-- name: InsertAutomationRuleHistory :exec
INSERT INTO automation_rule_history (organization_id, rule_id, action, actor_id, snapshot)
VALUES (@organization_id, @rule_id, @action, sqlc.narg(actor_id), @snapshot);

-- name: ListAutomationRuleHistory :many
SELECT h.id, h.action, h.snapshot, h.created_at, COALESCE(u.name, '')::text AS actor_name
FROM automation_rule_history h
LEFT JOIN users u ON u.id = h.actor_id
WHERE h.organization_id = @organization_id AND h.rule_id = @rule_id
ORDER BY h.created_at DESC, h.id DESC
LIMIT 50;

-- name: AutomationStats :one
SELECT count(*)::bigint AS total,
    count(*) FILTER (WHERE active)::bigint AS active,
    COALESCE(sum(send_count), 0)::bigint AS sent
FROM automation_rules
WHERE organization_id = @organization_id AND deleted_at IS NULL;

-- name: CountAutomationDrafts :one
SELECT count(*)::bigint FROM automation_runs WHERE organization_id = @organization_id AND status = 'draft';

-- name: InsertAutomationRun :execrows
-- Tekil anahtar (kural + run_key) aynı mükellefe aynı dönemde ikinci
-- çalışmayı engeller: çakışmada hiçbir şey yazılmaz.
INSERT INTO automation_runs (organization_id, rule_id, engagement_id, run_key, period, status, scheduled_for, vars,
    source_ref, note)
VALUES (@organization_id, @rule_id, sqlc.narg(engagement_id), @run_key, @period, @status, @scheduled_for, @vars,
    @source_ref, @note)
ON CONFLICT (organization_id, rule_id, run_key) DO NOTHING;

-- name: GetAutomationRunForUpdate :one
SELECT * FROM automation_runs WHERE organization_id = @organization_id AND id = @id FOR UPDATE;

-- name: FinishAutomationRun :one
UPDATE automation_runs
SET status = @status, subject = @subject, body = @body, channels = @channels::text[], result = @result, note = @note,
    mail_message_id = sqlc.narg(mail_message_id), sms_followup_at = sqlc.narg(sms_followup_at),
    resend_check_at = sqlc.narg(resend_check_at), sent_at = sqlc.narg(sent_at),
    decided_by = COALESCE(sqlc.narg(decided_by), decided_by), decided_at = COALESCE(sqlc.narg(decided_at), decided_at)
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: FinishAutomationFollowup :exec
UPDATE automation_runs
SET sms_followup_status = @sms_followup_status, result = @result
WHERE organization_id = @organization_id AND id = @id;

-- name: ClearAutomationResend :exec
UPDATE automation_runs SET resend_check_at = NULL
WHERE organization_id = @organization_id AND id = @id;

-- name: ListAutomationRuns :many
SELECT r.*, ru.name AS rule_name, ru.uuid AS rule_uuid,
    COALESCE(t.title, '')::text AS taxpayer_title, COALESCE(e.uuid::text, '')::text AS engagement_uuid
FROM automation_runs r
JOIN automation_rules ru ON ru.id = r.rule_id AND ru.organization_id = r.organization_id
LEFT JOIN engagements e ON e.organization_id = r.organization_id AND e.id = r.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE r.organization_id = @organization_id
    AND (sqlc.narg(rule_id)::bigint IS NULL OR r.rule_id = sqlc.narg(rule_id))
    AND (sqlc.narg(status)::text IS NULL OR r.status = sqlc.narg(status))
ORDER BY r.created_at DESC, r.id DESC
LIMIT @lim;

-- name: ListAutomationRunIDsByUUID :many
SELECT id FROM automation_runs
WHERE organization_id = @organization_id AND uuid = ANY(@uuids::uuid[]) AND status = 'draft';

-- name: ListAutomationEngagementIDs :many
SELECT id FROM engagements
WHERE organization_id = @organization_id AND uuid = ANY(@uuids::uuid[]);

-- name: ListAutomationTargets :many
-- Hedef mükellef dosyaları: ofisin etkin dosyaları, seçili hedefte yalnız
-- seçilenler.
SELECT e.id AS engagement_id
FROM engagements e
WHERE e.organization_id = @organization_id AND e.status = 'active'
    AND (NOT @only_selected::boolean OR e.uuid = ANY(@uuids::uuid[]))
ORDER BY e.id;

-- name: GetAutomationRecipient :one
SELECT e.id AS engagement_id, e.uuid AS engagement_uuid, t.id AS taxpayer_id, t.uuid AS taxpayer_uuid, t.title,
    COALESCE(t.contact_phone, '')::text AS contact_phone, COALESCE(t.contact_email::text, '')::text AS contact_email,
    o.name AS organization_name
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
JOIN organizations o ON o.id = e.organization_id
WHERE e.organization_id = @organization_id AND e.id = @engagement_id;

-- name: GetAutomationEngagementByUUID :one
SELECT id FROM engagements WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: GetAutomationReturnDue :one
-- Dönemin beyanı: hazırlanmış ya da verilmiş, ödenecek tutar ve ödendi mi.
SELECT payable_amount, paid_on, status
FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind AND period = @period;

-- name: CountAutomationOpenDocTasks :one
SELECT count(*)::bigint FROM document_tasks
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND status = 'open';

-- name: ListAutomationBankNames :many
SELECT DISTINCT name FROM cash_boxes
WHERE taxpayer_id = @taxpayer_id AND type = 'bank_account' AND name <> ''
ORDER BY name
LIMIT 5;

-- name: ListAutomationTaxpayerMembers :many
SELECT user_id FROM taxpayer_members WHERE taxpayer_id = @taxpayer_id ORDER BY user_id;

-- tenant-lint: org-wide otomasyon olay taraması (tahakkuk oluştu), ofisin bütün dosyaları
-- name: ListAutomationFiledReturns :many
SELECT tr.uuid, tr.engagement_id, tr.kind, tr.period, tr.payable_amount, tr.paid_on, tr.filed_at
FROM tax_returns tr
WHERE tr.organization_id = @organization_id AND tr.status = 'filed' AND tr.filed_at >= @since
ORDER BY tr.filed_at
LIMIT 500;

-- tenant-lint: org-wide otomasyon olay taraması (beyanname onaylandı), ofisin bütün dosyaları
-- name: ListAutomationCheckedReturns :many
SELECT tr.uuid, tr.engagement_id, tr.kind, tr.period, tr.payable_amount, tr.checked_at
FROM tax_returns tr
WHERE tr.organization_id = @organization_id AND tr.checked_at IS NOT NULL AND tr.checked_at >= @since
ORDER BY tr.checked_at
LIMIT 500;

-- tenant-lint: org-wide otomasyon olay taraması (belge 3 gündür gelmedi), ofisin bütün dosyaları
-- name: ListAutomationOverdueDocTasks :many
SELECT dt.uuid, dt.engagement_id, dt.period, dt.created_at
FROM document_tasks dt
WHERE dt.organization_id = @organization_id AND dt.status = 'open'
    AND dt.created_at <= @overdue_before AND dt.created_at >= @since
ORDER BY dt.created_at
LIMIT 500;

-- name: GetAutomationReturnPaid :one
SELECT (paid_on IS NOT NULL)::boolean AS paid FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: GetAutomationMailOpened :one
SELECT (opened_at IS NOT NULL)::boolean AS opened FROM mail_messages
WHERE organization_id = @organization_id AND id = @id;

-- tenant-lint: cross-org otomasyon zamanlayıcısı: etkin kuralı olan ofisler (ana anahtar açık)
-- name: ListAutomationActiveOrgs :many
SELECT DISTINCT r.organization_id
FROM automation_rules r
LEFT JOIN office_automation_settings s ON s.organization_id = r.organization_id
WHERE r.active AND r.deleted_at IS NULL AND COALESCE(s.enabled, TRUE)
ORDER BY r.organization_id;

-- name: ListActiveAutomationRules :many
SELECT * FROM automation_rules
WHERE organization_id = @organization_id AND active AND deleted_at IS NULL
ORDER BY id;

-- tenant-lint: cross-org otomasyon zamanlayıcısı: zamanı gelen çalışmalar
-- name: ListDueAutomationRuns :many
SELECT organization_id, id FROM automation_runs
WHERE status = 'scheduled' AND scheduled_for <= @now
ORDER BY scheduled_for, id
LIMIT @lim;

-- tenant-lint: cross-org otomasyon zamanlayıcısı: e-posta açılmadıysa SMS kontrolü
-- name: ListDueAutomationFollowups :many
SELECT organization_id, id FROM automation_runs
WHERE sms_followup_at IS NOT NULL AND sms_followup_status = '' AND sms_followup_at <= @now AND status = 'sent'
ORDER BY sms_followup_at, id
LIMIT @lim;

-- tenant-lint: cross-org otomasyon zamanlayıcısı: hâlâ ödenmediyse tekrar gönder
-- name: ListDueAutomationResends :many
SELECT organization_id, id FROM automation_runs
WHERE resend_check_at IS NOT NULL AND resend_check_at <= @now
ORDER BY resend_check_at, id
LIMIT @lim;

-- name: CountAutomationPaidVatReturn :one
SELECT count(*)::bigint FROM tax_returns
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = 'kdv1' AND period = @period
    AND paid_on IS NOT NULL;
