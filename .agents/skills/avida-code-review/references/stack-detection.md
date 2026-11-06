# Stack Detection

Before dispatching reviewers, detect the project stack so the right language/stack agents are launched. The orchestrator runs cheap filesystem checks at repo root.

## Detection rules

| Marker found at repo root | Language/stack agents to include |
|---|---|
| `go.mod` | `golang-pro` |
| `pyproject.toml`, `setup.py`, `requirements*.txt`, `Pipfile`, `poetry.lock` | `python-pro` |
| `package.json` **without** `tsconfig.json` | (skip frontend-specific language agent; treat as JS) |
| `package.json` **and** `tsconfig.json` | `typescript-pro` |
| `package.json` **and** any file under `src/` matching `*.tsx` or `*.jsx`, or `react` in `package.json` deps | `react-specialist` |
| Frontend stack detected (TypeScript or React above) | `frontend-developer` |
| Any of: `migrations/`, `*.sql`, `clickhouse/`, `db/migrations/`, repository uses SQL builder (`squirrel`, `sqlalchemy.text`, raw `asyncpg`, `pgx`, project Query Builder) | `sql-pro` |

## Avito PaaS markers (enrichment, not new agents)

If any of the following is found at repo root, the **stack is classified as Avito PaaS**:

- `app.toml`
- `auth.toml`
- `nfr.toml`
- `alert.yaml`
- `brief/` directory
- `db/changelog/` or `migrations/postgresql/`

Avito classification does **not** change the agent roster. Instead, the orchestrator appends Avito-specific reference files (`references/avito/*.md`) to each selected reviewer's shared context, per the mapping in `references/avito/README.md`:

| Avito file present | References to attach to which agents |
|---|---|
| `auth.toml` | `references/avito/security.md` → security-auditor |
| `nfr.toml` | `references/avito/nfr.md` → architect-reviewer, qa-expert |
| `db/changelog/` or `migrations/postgresql/` | `references/avito/migrations.md` → sql-pro |
| `brief/data-bus/` or `[[data-bus]]` in `app.toml` | `references/avito/databus.md` → fullstack-developer |
| `brief/qaas/` or `[[queues]]` in `app.toml` | `references/avito/queues.md` → fullstack-developer |
| `brief/rpc/` or `[[dependencies]]` with `brief_rpc` | `references/avito/brief.md` → architect-reviewer, fullstack-developer |
| `*.brief` with any `//brief:sensitive` annotation | `references/avito/sensitive-logging.md` → security-auditor |
| `alert.yaml` OR code with `log.`/`statsd.`/`graceful.` calls | `references/avito/observability.md` → qa-expert, security-auditor |
| `app.toml` + `brief/` OR `internal/generated/` / `__generated__/` | `references/avito/architecture.md` → architect-reviewer, golang-pro, typescript-pro, frontend-developer |
| Diff adds new RPC method, REST endpoint, data-bus / queues consumer | `references/avito/config-completeness.md` → architect-reviewer, fullstack-developer |

Always include `references/avito/README.md` once in the shared context for the orchestrator's own awareness.

The **non-language core set** is always launched:

- `architect-reviewer`
- `code-reviewer`
- `code-simplifier`
- `fullstack-developer`
- `qa-expert`
- `security-auditor`

## Implementation steps for the orchestrator

There is no shell — inspect the PR clone by **reading and globbing** files in its workspace,
not by running `ls` / `grep` / `test -f`.

1. Glob the repo root for the language markers: `go.mod`, `pyproject.toml`, `package.json`, `tsconfig.json`.
2. For frontend: check whether `package.json` exists and read it for a `"react"` dependency (or glob `src/` for `*.tsx` / `*.jsx`).
3. For SQL: glob for `migrations/`, top-level `*.sql`, or search the changed files for stack-specific markers (`squirrel`, `asyncpg`, `Query Builder`).
4. For Avito PaaS: probe (read/glob) for `app.toml`, `auth.toml`, `nfr.toml`, `alert.yaml`, `brief/`, `db/changelog/`, `migrations/postgresql/` at repo root. When any are present, classify the stack as Avito PaaS and resolve the per-agent reference attachments per the table above.
5. Always include the non-language core set.
6. Output the resolved list, e.g. `[architect-reviewer, code-reviewer, code-simplifier, fullstack-developer, qa-expert, security-auditor, python-pro, sql-pro]` plus the Avito attachments per agent if applicable.

## Manual override (focus)

A focus from the trigger/comment — `"review this branch as a frontend project"`,
`"only python and security"`, `"migrations only"` — is **authoritative**: skip detection and
keep only the roles serving the focus (plus the relevant core role). When the focus names
files/paths, also narrow the reviewed scope to them. Focus only ever *narrows* the review.

## Examples

- `service-ab-configurator` (Python + SQL backend) → core + `python-pro` + `sql-pro` (8 agents).
- `service-split-tests-go` (Go + SQL backend) → core + `golang-pro` + `sql-pro` (8 agents).
- `service-ab-frontend` (React + TS) → core + `typescript-pro` + `react-specialist` + `frontend-developer` (9 agents).
- Polyglot repo with both Go service and TS frontend → include both stacks (10+ agents). If parallel-dispatch limit is hit, prioritise the stack of the changed files in this diff.
