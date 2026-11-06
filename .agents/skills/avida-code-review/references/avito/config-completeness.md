# Config Completeness Gate

**Activates when**: the diff adds or changes a public RPC method, REST endpoint, Data Bus producer/consumer, or Queues consumer.

**Used by**: `architect-reviewer`, `fullstack-developer`.

## Rule

A new public entrypoint requires synchronised changes across the platform-config family. **Missing any of the required files for the entrypoint type is a valid finding** (severity P1 — operational gap that will be caught only in prod).

## What to check, by entrypoint type

### New RPC method (`brief/rpc/service.brief` or `brief/rpc/service/*.brief`)

Required updates:

| File | What must be there | Severity if missing |
|---|---|---|
| `auth.toml` | Method in a `[[policy]]` block with explicit `clients` (no wildcard) | **P0** — endpoint is unauthorized by default-deny |
| `nfr.toml` | `[[rpc]]` block with `latency`, `rpm`, `error_codes` or covered by `[default.all]` | P1 — no SLI for the new method |
| `app.toml` | Handler registered, dependencies declared if it calls external services | P1 — won't wire up |
| `alert.yaml` | Alert exists for error rate / latency on the path family | P2 — no monitoring on new surface |

### New REST endpoint

Same as RPC, plus check that path follows `http:METHOD:/path` form in `auth.toml`.

### New Data Bus consumer or producer

| File | What must be there |
|---|---|
| `app.toml` | `[[data-bus]]` entry with `schema`, `consumers`, `dlq` if relevant |
| `auth.toml` | `auth_events = true` + `events` policy listing the schema, if event-level auth is on |
| `nfr.toml` | `[[operation]]` block if consumer affects SLI |
| `alert.yaml` | Alert on consumer backlog / error rate |

### New Queues consumer

| File | What must be there |
|---|---|
| `app.toml` | `[[queues]]` entry with `schema`, `dlq`, `consumers` |
| `nfr.toml` | `[[operation]]` block |
| `alert.yaml` | Alert on backlog / consumer errors |

## How to verify

1. Detect new entrypoint in the diff:
   - new `rpc <name>(...)` line in `brief/rpc/*`
   - new `[[data-bus]]`, `[[queues]]`, or `[[http]]` block in `app.toml`
   - new handler registration in `cmd/` / `internal/` / `app.go`
2. Grep for the entrypoint name in the partner config files.
3. Missing reference → emit finding with `category: config_completeness` and `evidence.convention_source: references/avito/config-completeness.md`.

## What this is NOT

- Not a check that the *values* in those files are correct — for that use `migrations.md`, `nfr.md`, `security.md`, `observability.md`.
- Not a check that auth policy is sensible — that's `security.md`'s job.

This is the gate that catches the **forgotten file** problem (auth not added at all, NFR not added at all, alert not added at all).
