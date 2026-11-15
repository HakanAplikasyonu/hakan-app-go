-- name: InsertAIUsageEvent :exec
INSERT INTO ai_usage_events (
    organization_id, taxpayer_id, user_id, purpose, provider, model, is_fallback,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
    cost_usd, masked_values
) VALUES (
    sqlc.arg(organization_id), sqlc.narg(taxpayer_id), sqlc.narg(user_id),
    sqlc.arg(purpose), sqlc.arg(provider), sqlc.arg(model), sqlc.arg(is_fallback),
    sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens),
    sqlc.arg(cache_write_tokens), sqlc.narg(cost_usd), sqlc.arg(masked_values)
);

-- name: AIUsageReport :many
-- Office AI cost report: one row per month x taxpayer x model.
SELECT
    date_trunc('month', created_at AT TIME ZONE 'Europe/Istanbul')::date AS month,
    taxpayer_id,
    model,
    count(*)::bigint                                   AS calls,
    count(*) FILTER (WHERE is_fallback)::bigint        AS fallback_calls,
    count(*) FILTER (WHERE cost_usd IS NULL)::bigint   AS unpriced_calls,
    coalesce(sum(input_tokens), 0)::bigint             AS input_tokens,
    coalesce(sum(output_tokens), 0)::bigint            AS output_tokens,
    coalesce(sum(cache_read_tokens), 0)::bigint        AS cache_read_tokens,
    coalesce(sum(cache_write_tokens), 0)::bigint       AS cache_write_tokens,
    coalesce(sum(cost_usd), 0)::numeric(14,6)          AS cost_usd
FROM ai_usage_events
WHERE organization_id = sqlc.arg(organization_id)
  AND created_at >= sqlc.arg(from_at)
  AND created_at < sqlc.arg(to_at)
GROUP BY 1, 2, 3
ORDER BY 1, 2 NULLS FIRST, 3;
