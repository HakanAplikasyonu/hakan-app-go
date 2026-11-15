-- name: ListWarningCodes :many
SELECT code, section, default_severity, default_params
FROM warning_codes
WHERE cardinality(sqlc.arg(codes)::text[]) = 0 OR code = ANY(sqlc.arg(codes)::text[])
ORDER BY code;

-- name: ListWarningRuleConfigs :many
-- Office rows (engagement_id IS NULL) plus, when engagement_id is given,
-- that engagement's rows.
SELECT code, engagement_id, severity, enabled, params, updated_by, updated_at
FROM warning_rule_configs
WHERE organization_id = sqlc.arg(organization_id)
  AND (engagement_id IS NULL OR engagement_id = sqlc.narg(engagement_id)::bigint)
  AND (cardinality(sqlc.arg(codes)::text[]) = 0 OR code = ANY(sqlc.arg(codes)::text[]))
ORDER BY code, engagement_id NULLS FIRST;

-- name: UpsertOfficeWarningRule :exec
INSERT INTO warning_rule_configs (organization_id, engagement_id, code, severity, enabled, params, updated_by)
VALUES (sqlc.arg(organization_id), NULL, sqlc.arg(code), sqlc.narg(severity), sqlc.narg(enabled), sqlc.arg(params), sqlc.narg(updated_by))
ON CONFLICT (organization_id, code) WHERE engagement_id IS NULL
DO UPDATE SET severity = EXCLUDED.severity, enabled = EXCLUDED.enabled,
              params = EXCLUDED.params, updated_by = EXCLUDED.updated_by;

-- name: UpsertEngagementWarningRule :exec
INSERT INTO warning_rule_configs (organization_id, engagement_id, code, severity, enabled, params, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(engagement_id), sqlc.arg(code), sqlc.narg(severity), sqlc.narg(enabled), sqlc.arg(params), sqlc.narg(updated_by))
ON CONFLICT (organization_id, engagement_id, code) WHERE engagement_id IS NOT NULL
DO UPDATE SET severity = EXCLUDED.severity, enabled = EXCLUDED.enabled,
              params = EXCLUDED.params, updated_by = EXCLUDED.updated_by;

-- name: DeleteOfficeWarningRule :execrows
DELETE FROM warning_rule_configs
WHERE organization_id = sqlc.arg(organization_id) AND engagement_id IS NULL AND code = sqlc.arg(code);

-- name: DeleteEngagementWarningRule :execrows
DELETE FROM warning_rule_configs
WHERE organization_id = sqlc.arg(organization_id) AND engagement_id = sqlc.arg(engagement_id) AND code = sqlc.arg(code);
