# Database rules

## Engine

- **PostgreSQL** — primary system of record
- **Redis** — cache, queue (Asynq), ephemeral state — not authoritative business data
- **Object binary** — S3 (SeaweedFS locally); metadata may live in Postgres

## Access path

```
UseCase → Repository Interface → Repository Impl (sqlc / pgx) → PostgreSQL
```

### Forbidden

- SQL inside handlers / controllers
- Direct `*pgxpool.Pool` use in use cases (outside repositories)
- ORM-managed schema (GORM, etc.)
- Manual production schema changes outside migrations

## Migrations (mandatory)

- Tool: **golang-migrate**
- Location: `migrations/`
- Naming: **sequential** — `00000N_description.up.sql` / `.down.sql`
  - Create with: `make migrate-create NAME=<description>` (uses `-seq`)
  - Do **not** use timestamp naming
- Every schema change ships as a migration
- Both `up` and `down` are required (`down` data-loss risk must be obvious from the SQL)

### Workflow

1. `make migrate-create NAME=...`
2. Write up + down SQL
3. `make migrate-up` (local)
4. `make sqlc`
5. Update repository / tests
6. Include migration + generated `db` package in the PR

### Baseline squash

Historical migrations may be squashed into a clean `000001`–`000019` baseline that reflects the **current** schema (no create-then-alter patches). After a squash, existing local databases must be reset — not migrated incrementally from old version numbers:

```bash
# drops all tables + schema_migrations; then re-applies migrations
psql "$DB_URL" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
make migrate-up
make create-super-admin
```

## SQLC

- Query sources: `internal/database/queries/*.sql`
- Generated package: `internal/database/db` (committed; **do not edit by hand**)
- Catalog stubs for codegen only: `internal/database/schema/` (not applied by migrate)
- Config: `sqlc.yaml`
- Regenerate: `make sqlc`

## Schema design principles

| Concern | Rule |
|---------|------|
| Internal PK | `BIGSERIAL` column `id` |
| Public id | `uuid` with `gen_random_uuid()` (pgcrypto) where exposed to API/clients |
| Timestamps | `timestamptz` — `created_at`, `updated_at` |
| Auto `updated_at` | `BEFORE UPDATE … EXECUTE FUNCTION set_updated_at()` (function from `000002`) |
| Soft delete | `deleted_at timestamptz NULL` when needed |
| RBAC | Global users; roles via `user_roles`; permissions via `role_permissions`. System roles (`is_system=true`) are immutable. `super_admin` is assigned by CLI, not a user column. |
| Money | `numeric` — never `float` / `double precision` |
| Enums | Postgres enum or `text` + check; keep app constants in sync |
| Indexes | Add for real query paths; avoid premature indexes |
| Foreign keys | Required unless explicitly justified |

## Naming

| Object | Convention |
|--------|------------|
| Tables | `snake_case`, plural (`users`, `roles`, `notifications`) |
| Columns | `snake_case` |
| PK | `id` |
| FK | `<singular>_id` (`user_id`, `role_id`) |
| Unique indexes | `uq_<table>_<cols>` |
| Indexes | `idx_<table>_<cols>` |

## Transactions

- Multi-write use cases own the transaction boundary
- Prefer single-responsibility repository methods
- Pass transactions via `context` / `WithTx`

## Tenant permissions and office roles (HAKAN, issue #5)

Every office has its own copies of the template tenant roles (owner, admin,
accountant, member, intern), made when the office is created
(`trg_organizations_copy_tenant_roles`). Offices edit their copies and add
custom roles; templates (`roles.organization_id IS NULL`) are only the source
for new offices.

When a migration adds a `tenant.*` permission:

- The `owner` role of every office gets it automatically
  (`trg_permissions_grant_tenant_owner`).
- Grant it to any other role with
  `SELECT grant_tenant_role_permission('<role_slug>', '<permission_slug>');`
  which writes the template **and** every office copy of that system role.
  Inserting into `role_permissions` for the template only does not reach
  existing offices.

## Row level security (HAKAN, issue #6)

Scoped tables (`tenant_tables.txt`, `taxpayer_tables.txt`) have Postgres RLS
policies for two scoped roles, `hakan_rls_app` and `hakan_rls_worker` (no
BYPASSRLS). The connection user owns the tables and is not restricted (tables
are not FORCE'd), so migrations and unscoped code paths work unchanged.
Code that wants the database to enforce isolation runs in
`rls.WithScope(ctx, pool, rls.FromRequest(ctx), fn)` (`internal/platform/rls`):
`SET LOCAL ROLE` plus `SET LOCAL app.organization_id / app.user_id /
app.taxpayer_ids / app.engagements_read_all`.

A migration that creates a scoped table must also call one template:

```sql
SELECT rls_apply_org('my_table');         -- organization_id
SELECT rls_apply_engagement('my_table');  -- organization_id + engagement_id
SELECT rls_apply_taxpayer('my_table');    -- taxpayer_id
SELECT rls_apply_platform('my_table');    -- no rows for scoped roles
```

`TestRLSCoversScopedTables` fails while a listed table has no policy.

## Migration safety

- Expand/Contract for breaking drops (two-step)
- Plan lock-heavy `ALTER` carefully for production
- Backfills may be separate migrations or jobs
- Seeds in migrations only for reference/system data — no demo users

## Redis rules

- Not the business source of truth
- Cache / queue / rate-limit / ephemeral session-style data
- Key prefix: `app:<env>:...`
- TTL required for cache/session-like keys

## Review checklist

- [ ] up + down migration present?
- [ ] FK / indexes correct?
- [ ] `make sqlc` regenerated and committed?
- [ ] Sensitive columns handled correctly?
- [ ] Rollback impact understood?
