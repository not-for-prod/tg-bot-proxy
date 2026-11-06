# PostgreSQL Migrations (Avito PaaS)

**Activates when**: `db/changelog/` or `migrations/postgresql/` in diff.

**Used by**: `sql-pro`.

Avito-specific checks beyond `avito lint`. Universal sql-pro reviewer doesn't know these conventions.

## Target files

- Classic migrator: `db/changelog/master/*.sql`, `db/changelog/master-repeatable/*.sql`, `db/changelog/indexer/*.sql`, `db/fixtures/master/*.sql`
- Unified migrator: `migrations/postgresql/master/*.sql`, `migrations/postgresql/master-repeatable/*.sql`, `migrations/postgresql/fixtures/master/*.sql`

Out of scope: MongoDB, CockroachDB, Data Bus / Queues schemas.

## CRITICAL checks

### 1. Timeouts on heavy operations

Without `lock_timeout` / `statement_timeout`, ALTER/DROP/mass UPDATE can lock a table for an unpredictable time.

| Operation | Timeout | Recommendation |
|---|---|---|
| `ALTER TABLE ... ADD COLUMN ... DEFAULT` | `lock_timeout` | Don't use DEFAULT in ADD COLUMN for PG < 11 |
| `ALTER TABLE ... DROP COLUMN` | `lock_timeout` | `"1s"` — `"5s"` |
| `ALTER TABLE ... ALTER COLUMN TYPE` | `lock_timeout` | depends on table size |
| Mass UPDATE (~1k rows) | `statement_timeout` | `"5s"` |
| Mass UPDATE (~50k rows) | `statement_timeout` | `"60s"` |
| Mass UPDATE (~100k rows) | `statement_timeout` | `"180s"` |

```sql
-- OK: PG >= 12, SET LOCAL inside transaction
BEGIN;
SET LOCAL lock_timeout = "1s";
ALTER TABLE public.orders ADD COLUMN status text;
COMMIT;

-- BAD: no timeout, NOT NULL DEFAULT rewrites the entire table
ALTER TABLE public.big_table ADD COLUMN new_column text NOT NULL DEFAULT 'default';
```

`lock_timeout` is included in `statement_timeout` — setting `lock_timeout >= statement_timeout` is pointless.

### 2. Blocking operations — CREATE INDEX

`CREATE INDEX` without `CONCURRENTLY` blocks writes. `CONCURRENTLY` cannot run inside a transaction, so it cannot be applied by the migrator.

Pattern for big tables:

1. Migration (idempotent, no CONCURRENTLY): `CREATE INDEX IF NOT EXISTS orders_user_id_idx ON public.orders (user_id);`
2. Manually apply in prod with CONCURRENTLY before the migration is rolled out.
3. Migration becomes a no-op in prod (index already exists) and creates it in other envs.

Findings:

- `CREATE INDEX` on a table likely > 1GB without the manual-CONCURRENTLY plan → **P1**.
- No `IF NOT EXISTS` on `CREATE INDEX` → P2 (idempotency).

### 3. ADD COLUMN with DEFAULT (PG < 11)

```sql
-- CRITICAL on PG < 11: full table rewrite while locked
ALTER TABLE public.orders ADD COLUMN status text NOT NULL DEFAULT 'pending';
```

Safe pattern: split into three steps — add nullable column, backfill in batches, then SET NOT NULL + SET DEFAULT.

PG 11+ with constant DEFAULT: safe. PG 11+ with volatile DEFAULT (`now()`): still rewrites.

### 4. ALTER COLUMN TYPE on big tables

Same problem — blocks and rewrites. For tables > 1GB use the shadow-column pattern (new column + sync trigger + batch backfill + atomic swap).

### 5. Mass data changes — batching mandatory

Mass `UPDATE`/`DELETE` in a single transaction degrades replicas, balloons WAL, creates dead tuples.

Pattern: batch loop with `pg_sleep(0.1-0.5s)` between transactions, batch size 1k-100k (lower with DWH triggers).

### 6. DROP COLUMN — backward compatibility

```sql
-- DANGEROUS without staged removal
ALTER TABLE public.orders DROP COLUMN old_status;
```

Required staged removal:
1. Code stops using the column (deploy, observe).
2. Migration deletes it.

Findings:
- Code in the same PR still references the dropped column → **P0**.
- `SELECT *` against the table elsewhere → P1 (will break the moment column is gone).

## WARNING checks

### 7. Idempotency

`IF NOT EXISTS` / `IF EXISTS` on CREATE/DROP. Repeatable migrations use `CREATE OR REPLACE`. Triggers re-created with `DROP TRIGGER IF EXISTS ... CREATE TRIGGER ...`.

### 8. Naming

- Files: `YYYYMMDD-N-description.sql` or `YYYYMMDDHHMMSS_description.sql`.
- Indexes: `{table}_{columns}_{type}` (`orders_user_id_idx`, `orders_order_number_ux`).
- Constraints: `{table}_{column}_{type}` (`orders_user_id_fk`, `orders_status_check`).

### 9. Sampler integration

If service uses sampler, sampler config must include `maintenance.changelog` (classic) or `schema_migrations` (unified). Otherwise sampler restore breaks the migrator (re-runs already-applied migrations).

### 10. FK / UNIQUE / CHECK on big tables

Use `NOT VALID` + later `VALIDATE CONSTRAINT` to avoid the full-table scan lock.

```sql
ALTER TABLE public.orders ADD CONSTRAINT orders_status_check
CHECK (status IN ('pending', 'completed', 'cancelled')) NOT VALID;
-- later
ALTER TABLE public.orders VALIDATE CONSTRAINT orders_status_check;
```

### 11. `--@SKIP` directive

`--@SKIP` skips migration in prod, requires manual application. Flag any new `--@SKIP` migration if the manual-application plan is not documented in the PR description.

### 12. UNIQUE constraint via CONCURRENTLY

```sql
CREATE UNIQUE INDEX CONCURRENTLY orders_order_number_ux ON public.orders (order_number);
ALTER TABLE public.orders ADD CONSTRAINT orders_order_number_uk UNIQUE (order_number) USING orders_order_number_ux;
```

The blocking form (`ALTER TABLE ... ADD CONSTRAINT ... UNIQUE`) is a P1 finding on a non-trivial table.

## Anti-patterns

- DDL in application runtime code (`app.db.Exec("CREATE TABLE ...")`) — P0.
- `DROP TABLE` with FK without cascade plan — P1.
- Mega-migration files (one file with 5000 lines of mixed DDL/DML) — P2; recommend splitting.

## Quick checklist

1. Timeouts present on ALTER/DROP/mass ops?
2. `CREATE INDEX` on big tables — manual CONCURRENTLY plan documented?
3. `ADD COLUMN` with DEFAULT — PG version + volatility checked?
4. `ALTER COLUMN TYPE` on big tables — shadow-column pattern?
5. Mass data change — batched with pauses?
6. `DROP COLUMN` — code stopped using it in a previous deploy?
7. `IF EXISTS` / `IF NOT EXISTS` for idempotency?
8. Naming follows `{table}_{column}_{type}`?
9. Sampler config includes migration tables?
10. FK/UNIQUE/CHECK on big tables — `NOT VALID` + VALIDATE?
