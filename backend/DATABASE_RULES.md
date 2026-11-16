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

## Row level security (HAKAN, issues #6 and #166)

Scoped tables (`tenant_tables.txt`, `taxpayer_tables.txt`) have Postgres RLS
policies for two scoped roles, `hakan_rls_app` and `hakan_rls_worker` (no
BYPASSRLS). Code that wants the database to enforce isolation runs in
`rls.WithScope(ctx, pool, rls.FromRequest(ctx), fn)` (`internal/platform/rls`):
`SET LOCAL ROLE` plus `SET LOCAL app.organization_id / app.user_id /
app.taxpayer_ids / app.engagements_read_all`.

**Runtime role (issue #166).** Every application connection runs as
`hakan_rls_runtime` (`database.NewPostgresPool`, `DB_RLS_RUNTIME`, default
on; tests via `testdb` too). It is not a superuser, has no BYPASSRLS and owns
nothing, so RLS applies to every query. FORCE ROW LEVEL SECURITY alone would
not do this: it only affects the table owner, and a superuser (the official
postgres image's `POSTGRES_USER`, CI's `postgres`) skips it. Migrations run
as the owner and are not affected.

Each RLS table is in one of two states:

| State | Policies for `hakan_rls_runtime` | Unscoped query |
|---|---|---|
| in transition | `rls_legacy` (`USING true`) | works as before |
| enforced (`rls_enforced.txt`) | none; FORCE + owner `rls_system` policy | sees nothing, cannot write |

**Owner forced everywhere (issue #302, migration 000086).** Every table with
an `organization_id` or `taxpayer_id` column also has FORCE and the owner's
`rls_system` policy (`rls_owner_force`, called by every `rls_apply_*`
template), in transition or enforced. The runtime role is not affected (it is
not the owner); a non-superuser owner sees and writes those tables only
inside `rls.System` (`app.system`). A data migration touching any such table
as a non-superuser owner must `SET LOCAL app.system = 'on'` first. The
exception list is empty since #307; a new exception goes, with its reason,
into `rlsInvariantExceptions` (`rls_invariant_db_test.go`).

Enforced tables answer only inside `rls.WithScope` or `rls.System(ctx, pool,
"reason", fn)`, the deliberate bypass for work that is not done on behalf of
one tenant (cross-tenant workers, the demo seed, maintenance). Foreign key
cascades still delete enforced rows.

A migration that creates a scoped table must also call one template (the new
table starts in transition):

```sql
SELECT rls_apply_org('my_table');         -- organization_id
SELECT rls_apply_engagement('my_table');  -- organization_id + engagement_id
SELECT rls_apply_taxpayer('my_table');    -- taxpayer_id
SELECT rls_apply_platform('my_table');    -- no rows for scoped roles
SELECT rls_apply_org_shared('my_table');  -- organization_id, NULL = platform row
```

`rls_apply_org_shared` (migration 000087, #307) is for tables whose
organization_id NULL rows belong to the platform (system roles, global Excel
templates, office-less IO engine jobs): a scope reads its own office's rows
and the NULL rows, writes only its own office's rows; a NULL row is written
only through `rls.System` (or unscoped, while the table is in transition).
List the table in `internal/database/shared_tables.txt`
(`TestSharedTablesPolicies`).

`notifications` and `activity_events` (migration 000088) add a user
condition on the office-less rows: a scope reads its office's rows and only
its own user's office-less rows (notifications: also user-less ones), and
writes only its office's rows. Code that notifies an office from the
mükellef panel writes inside `rls.ForOffice` (doctasks `notifyOffice`).

How code joins a scope: `database.NewQueries(pool)` (and `database.Ambient`)
run each statement on the transaction carried by the context, so a
repository called inside `WithScope` / `System` stays in that scope;
`Ambient.Begin` opens a savepoint there instead of a new, unscoped
connection (never call `pool.Begin` in code that may run in a scope; use
`database.Begin` / `database.BeginFunc` / `database.QueriesFor`).

**Every tenant request runs in a scope** (`DB_RLS_REQUEST_SCOPE`, default
on): `RequireOrganization` opens `rls.Middleware` for the office (taxpayers:
the user's visible engagements), `RequireEngagement` narrows it to the
engagement's taxpayer (`rls.Narrow`). Writes are buffered and roll back on
4xx/5xx; GET/HEAD stream. Inside the scope every RLS table, enforced or in
transition, is filtered by the scope policies.

**Taxpayer (mükellef panel) requests** run in a scope too: `RequireTaxpayer`
opens it for the user and the member taxpayer, with no office. Office tables
the panel reads (engagements, upload jobs, receipt captures, document tasks
and uploads, onboarding requests) carry `rls_taxpayer_party` (migration
000080): rows of the scope's own taxpayer, only when the scope has no
office, so office scopes are unchanged. A new office table the panel reads
needs `SELECT rls_apply_taxpayer_party('my_table');` and an entry in
`taxpayerParty` (`rls_templates_db_test.go`).

**Workers.** A job working for one office puts its own transactions under
`rls.Job(orgID, taxpayerID...)` with `rls.Apply(ctx, tx, scope)` (worker
role), skipped when `rls.InScope(ctx)` (already in a request scope). Jobs
that scan every office (reminders, offline sweeps) run in one `rls.System`
transaction with a reason. Questions that are not one tenant's ("which
office serves this taxpayer now") read through `rls.System`.

**Side effects (issue #311).** A write the request can do without — a
notification, an activity row, a review-queue submission for a low-confidence
OCR field, a data warning that only adds information — runs through
`database.BestEffort(ctx, "name", fn)`: in a savepoint of the request's
transaction, so its failure (an RLS refusal, a constraint) is rolled back
alone, logged (`side_effect_failed`) and counted
(`side_effect_failures_total{name}`) while the request commits its main work.
A failed statement outside a savepoint aborts the whole transaction (25P02).
Money, journal entries, approvals and the record the request is about are
never side effects. `notifications.Enqueue` and `activity.Recorder` already do
this for their callers.

Exceptions, on purpose:
- Secrets (`secret_records`, platform) are read and written by the vault on
  its own connection, never in a tenant scope.
- Office-wide facts that do not depend on the viewer (the bill's active
  taxpayer count) read through `rls.System` with a reason.
- Never nest `rls.System` inside a scope on the same transaction: its
  `SET LOCAL ROLE NONE` would outlive a released savepoint.

To enforce a table group: move every access path (API, worker, seed) into
`WithScope` or `System`, then `SELECT rls_enforce('my_table');` in a
migration, add the table to `internal/database/rls_enforced.txt` and its row
inserter to `leakFixtures` (`rls_enforcement_db_test.go`). A data migration
touching an enforced table as a non-superuser owner must
`SET LOCAL app.system = 'on'` first.

Tests: `TestScopedPoliciesMatchTemplates` (every listed table carries
exactly its template policy, nothing wider; the template behaviour is tested
in `internal/platform/rls`), `TestRLSCoversScopedTables` (every listed table has a policy),
`TestRLSEnforcement` (every RLS table is in one state, the list matches),
`TestEnforcedTablesDoNotLeak` (per enforced table: unscoped sees and writes
nothing, office A sees and writes only its own rows, System sees all, the
cascade still works), `TestRLSInvariant` (every table with an office or
taxpayer column: RLS on, FORCE on, a policy and `rls_system`, or a listed
exception).

**Write sweep** (`internal/httpserver/write_sweep_db_test.go`, #302): every
POST / PUT / PATCH / DELETE under `/v1/taxpayer/` and `/v1/tenant/` in
`openapi.yaml` is called once through the real server, on the demo office and
in the production request scope, as the demo taxpayer user and the office
owner; any 5xx fails (a 503 only where the case names the unavailable
external service). A new write endpoint fails the test until it has a case.

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
