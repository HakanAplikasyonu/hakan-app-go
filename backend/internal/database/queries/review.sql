-- İnceleme kuyruğu (issue #14).

-- name: SupersedePendingSuggestions :many
-- Aynı hedef alanın bekleyen önerisi yenisiyle değişir.
UPDATE suggestions
SET status = 'superseded', decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND kind = @kind AND target_type = @target_type
  AND target_id IS NOT DISTINCT FROM sqlc.narg(target_id)::bigint
  AND target_field = @target_field AND status = 'pending'
RETURNING id, confidence;

-- name: InsertSuggestion :one
INSERT INTO suggestions (
    organization_id, engagement_id, taxpayer_id, kind, target_type, target_id, target_field, period,
    proposed_value, confidence, rationale, origin, source_entity_type, source_entity_id,
    rule_ref, model, ai_usage_event_id, inputs
) VALUES (
    @organization_id, @engagement_id, @taxpayer_id, @kind, @target_type, sqlc.narg(target_id), @target_field, sqlc.narg(period),
    @proposed_value, @confidence, @rationale, @origin, sqlc.narg(source_entity_type), sqlc.narg(source_entity_id),
    sqlc.narg(rule_ref), sqlc.narg(model), sqlc.narg(ai_usage_event_id), @inputs
)
RETURNING *;

-- name: GetSuggestionForUpdate :one
SELECT * FROM suggestions
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
FOR UPDATE;

-- name: GetSuggestion :one
SELECT * FROM suggestions
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: UpdateSuggestionDecision :one
UPDATE suggestions
SET status = @status, applied_value = sqlc.narg(applied_value), previous_value = sqlc.narg(previous_value),
    rule_candidate = @rule_candidate, decided_by = sqlc.narg(decided_by), decided_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: InsertSuggestionDecision :exec
INSERT INTO suggestion_decisions (
    organization_id, engagement_id, suggestion_id, action, actor_user_id, reason, value, confidence, threshold
) VALUES (
    @organization_id, @engagement_id, @suggestion_id, @action, sqlc.narg(actor_user_id), @reason,
    sqlc.narg(value), @confidence, sqlc.narg(threshold)
);

-- name: ListSuggestionDecisions :many
SELECT d.action, d.reason, d.value, d.confidence, d.threshold, d.created_at, u.uuid AS actor_uuid
FROM suggestion_decisions d
LEFT JOIN users u ON u.id = d.actor_user_id
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id AND d.suggestion_id = @suggestion_id
ORDER BY d.id;

-- tenant-lint: org-wide ofis inceleme kuyruğu; read_all yoksa engagement_assignments ile süzülür
-- name: ListTenantSuggestions :many
SELECT s.*, e.uuid AS engagement_uuid
FROM suggestions s
JOIN engagements e ON e.organization_id = s.organization_id AND e.id = s.engagement_id
WHERE s.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR s.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR s.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR s.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(rule_candidate)::bool IS NULL OR s.rule_candidate = sqlc.narg(rule_candidate)::bool)
  AND (sqlc.narg(period)::text IS NULL OR s.period = sqlc.narg(period)::text)
ORDER BY CASE WHEN s.status = 'pending' THEN s.confidence END ASC NULLS LAST, s.updated_at DESC, s.id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofis inceleme kuyruğu sayımı; ListTenantSuggestions ile aynı süzgeç
-- name: CountTenantSuggestions :one
SELECT count(*)::bigint
FROM suggestions s
WHERE s.organization_id = @organization_id
  AND (sqlc.narg(engagement_id)::bigint IS NULL OR s.engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (@all_engagements::bool OR s.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR s.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(rule_candidate)::bool IS NULL OR s.rule_candidate = sqlc.narg(rule_candidate)::bool)
  AND (sqlc.narg(period)::text IS NULL OR s.period = sqlc.narg(period)::text);

-- name: ListReviewSettings :many
-- Ofis satırları ve, engagement verilirse, onun satırları.
SELECT engagement_id, kind, auto_accept, threshold, updated_at
FROM review_settings
WHERE organization_id = @organization_id
  AND (engagement_id IS NULL OR engagement_id = sqlc.narg(engagement_id)::bigint)
ORDER BY kind, engagement_id NULLS FIRST;

-- name: UpsertOfficeReviewSetting :exec
INSERT INTO review_settings (organization_id, engagement_id, kind, auto_accept, threshold, updated_by)
VALUES (@organization_id, NULL, @kind, sqlc.narg(auto_accept), sqlc.narg(threshold), sqlc.narg(updated_by))
ON CONFLICT (organization_id, kind) WHERE engagement_id IS NULL
DO UPDATE SET auto_accept = EXCLUDED.auto_accept, threshold = EXCLUDED.threshold, updated_by = EXCLUDED.updated_by;

-- name: UpsertEngagementReviewSetting :exec
INSERT INTO review_settings (organization_id, engagement_id, kind, auto_accept, threshold, updated_by)
VALUES (@organization_id, @engagement_id, @kind, sqlc.narg(auto_accept), sqlc.narg(threshold), sqlc.narg(updated_by))
ON CONFLICT (organization_id, engagement_id, kind) WHERE engagement_id IS NOT NULL
DO UPDATE SET auto_accept = EXCLUDED.auto_accept, threshold = EXCLUDED.threshold, updated_by = EXCLUDED.updated_by;

-- name: DeleteOfficeReviewSetting :execrows
DELETE FROM review_settings
WHERE organization_id = @organization_id AND engagement_id IS NULL AND kind = @kind;

-- name: DeleteEngagementReviewSetting :execrows
DELETE FROM review_settings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND kind = @kind;

-- tenant-lint: org-wide eşik etkisi özeti (#333); yalnızca sayım, read_all yoksa engagement_assignments ile süzülür
-- name: ReviewThresholdImpact :one
-- Son @months ayda karara bağlanmış (bekleyen ve yerine yenisi gelen hariç) öneriler.
SELECT count(*)::bigint AS decided,
       count(*) FILTER (WHERE s.confidence >= @threshold::numeric)::bigint AS would_auto_accept,
       count(*) FILTER (WHERE s.confidence >= @threshold::numeric
                          AND s.status IN ('corrected', 'rejected') AND s.decided_by IS NOT NULL)::bigint AS would_auto_accept_then_corrected,
       count(*) FILTER (WHERE s.status = 'auto_accepted')::bigint AS currently_auto_accepted,
       count(*) FILTER (WHERE s.confidence < @threshold::numeric)::bigint AS would_go_to_queue
FROM suggestions s
WHERE s.organization_id = @organization_id
  AND (@all_engagements::bool OR s.engagement_id IN (
        SELECT a.engagement_id FROM engagement_assignments a
        WHERE a.organization_id = @organization_id AND a.user_id = @user_id))
  AND s.status NOT IN ('pending', 'superseded')
  AND s.decided_at IS NOT NULL
  AND s.decided_at >= NOW() - make_interval(months => @months::int)
  AND (sqlc.narg(kind)::text IS NULL OR s.kind = sqlc.narg(kind)::text);
