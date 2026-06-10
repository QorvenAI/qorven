# Database — App Migrations, Scoping, Access

## App-scoped migrations

An app declares `migrations_dir: migrations` in `app.yaml`. Put numbered SQL files there; the runtime applies them on `install_app`.

- Files: `001_create_tables.up.sql`, `002_add_column.up.sql`, … (zero-padded, ascending). The first is `001`.
- Always `CREATE TABLE IF NOT EXISTS` so re-install is idempotent.
- **Prefix your tables with the app slug** to avoid collisions: `todo_app_items`, not `items`.
- When you change the schema later, ADD a new numbered migration (`002_…`) — never edit an already-applied one.

```sql
-- 001_create_tables.up.sql
CREATE TABLE IF NOT EXISTS todo_app_items (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    name       TEXT NOT NULL,
    done       BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

> Core Qorven migrations live in `backend/migrations/` and follow the same numbering, but `001_schema.up.sql` there is the whole base schema and is **never** edited — new core columns get a new numbered file. App migrations are separate and live inside the app.

## Scoping — multi-tenant and per-user

- **Tenant:** the default single-tenant id is `00000000-0000-0000-0000-000000000001` (`defaultTenant`). Stamp `tenant_id` on rows and filter by it, so the app is multi-tenant-safe.
- **Per-user:** if data belongs to a user, add `user_id UUID` and scope by it (and include it in any UNIQUE constraint, e.g. `UNIQUE (tenant_id, user_id, key)`). For a single-user self-hosted app this is still good hygiene.
- **App scope** (`scope: workspace|agent|team` in the manifest) controls who the app instance belongs to — independent of row-level tenant/user scoping.

## Accessing data

- **From tool scripts:** use `psql "$QORVEN_DB_DSN"` (the DSN is injected into the script env). Or keep simple state in a JSON file written atomically (`jq … > f.tmp && mv f.tmp f`).
- **From Go (core/handlers):** `gw.db.Pool.Query/QueryRow/Exec(ctx, sql, args...)` (pgxpool). Always guard `if gw.db == nil`, scope by `tenant_id`, and sanitize errors with `sanitizeError(err)` rather than returning raw `err.Error()`.

## Rules recap
1. Numbered, ascending, zero-padded migration files; first is `001`; never edit an applied one.
2. `CREATE TABLE IF NOT EXISTS`, slug-prefixed table names.
3. Always carry + filter `tenant_id`; add `user_id` for per-user data.
4. Parameterize all queries; never string-concat user input into SQL.
