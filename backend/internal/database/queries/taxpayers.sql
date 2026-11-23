-- Taxpayers (issue #3): platform düzeyinde global mükellef kimliği.
-- taxpayers bir ofise ait değildir; tenant_tables.txt'de yer almaz. Ofis
-- tarafı mükellefe yalnızca aktif ve erişilebilir bir engagement üzerinden
-- ulaşır (GetTaxpayerForOfficeMember).

-- name: CreateTaxpayer :one
INSERT INTO taxpayers (kind, tax_id, title, tax_office, address, activity_code)
VALUES (sqlc.arg(kind), sqlc.arg(tax_id), sqlc.arg(title), sqlc.arg(tax_office),
        sqlc.arg(address), sqlc.arg(activity_code))
RETURNING *;

-- name: GetTaxpayerByTaxID :one
SELECT * FROM taxpayers WHERE tax_id = sqlc.arg(tax_id);

-- name: GetTaxpayerByID :one
SELECT * FROM taxpayers WHERE id = sqlc.arg(id);

-- name: ListTaxpayers :many
SELECT * FROM taxpayers
WHERE (sqlc.narg(q)::text IS NULL
       OR tax_id = sqlc.narg(q)
       OR title ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountTaxpayers :one
SELECT COUNT(*)::bigint FROM taxpayers
WHERE (sqlc.narg(q)::text IS NULL
       OR tax_id = sqlc.narg(q)
       OR title ILIKE '%' || sqlc.narg(q) || '%');

-- name: UpdateTaxpayerProfile :one
UPDATE taxpayers
SET title      = COALESCE(sqlc.narg(title), title),
    tax_office = COALESCE(sqlc.narg(tax_office), tax_office),
    address    = COALESCE(sqlc.narg(address), address),
    activity_code = COALESCE(sqlc.narg(activity_code), activity_code)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;

-- GetTaxpayerForOfficeMember is the office-side access rule for taxpayer
-- data (docs/01 §2.3): the taxpayer is returned only through an ACTIVE
-- engagement of this organization, and only to a member assigned to it or
-- one whose role grants tenant.engagements.read_all (read_all = true). An
-- ended engagement, another office's engagement or a missing assignment all
-- return no row.
-- name: GetTaxpayerForOfficeMember :one
SELECT t.*
FROM taxpayers t
INNER JOIN engagements e ON e.taxpayer_id = t.id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND t.id = sqlc.arg(taxpayer_id)
  AND e.ended_at IS NULL
  AND (
      sqlc.arg(read_all)::boolean
      OR EXISTS (
          SELECT 1 FROM engagement_assignments a
          WHERE a.organization_id = e.organization_id
            AND a.engagement_id = e.id
            AND a.user_id = sqlc.arg(user_id)
      )
  );
