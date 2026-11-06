# Avito PaaS Enrichment

Avito-specific review checks layered on top of the universal agent roster. Each reference here is loaded **only** when the orchestrator detects the relevant Avito markers in the repo (see `../stack-detection.md`).

Universal agents stay platform-agnostic; this directory carries Avito conventions that the universal agents would otherwise miss.

## Files

| File | Triggered by | Used by |
|---|---|---|
| `architecture.md` | `app.toml` + `brief/` or `internal/generated/` | architect-reviewer, golang-pro, typescript-pro, frontend-developer |
| `config-completeness.md` | New public RPC/REST handler or consumer in diff | architect-reviewer, fullstack-developer |
| `migrations.md` | `db/changelog/`, `migrations/postgresql/` | sql-pro |
| `databus.md` | `[[data-bus]]` in `app.toml`, `brief/data-bus/` | fullstack-developer |
| `queues.md` | `[[queues]]` in `app.toml`, `brief/qaas/` | fullstack-developer |
| `security.md` | `auth.toml` | security-auditor |
| `sensitive-logging.md` | `//brief:sensitive` annotations in `brief/` | security-auditor |
| `observability.md` | Logging/metrics/tracing calls, `alert.yaml`, `app.toml [[dependencies]]` | qa-expert, security-auditor |
| `nfr.md` | `nfr.toml` | architect-reviewer, qa-expert |
| `brief.md` | `brief/rpc/`, `brief/rpc/clients/` | architect-reviewer, fullstack-developer |
| `tooling.md` | Avito stack detected | code-reviewer, code-simplifier, golang-pro, fullstack-developer |

## Loading rule

If `stack-detection.md` classifies the repo as **Avito stack** (any of `app.toml`, `auth.toml`, `nfr.toml`, `brief/`, `db/changelog/`, `alert.yaml` present at repo root), every selected reviewer agent additionally loads the references mapped to it above.

Reviewers treat these references as authoritative project conventions, equivalent to `AGENTS.md` / `CLAUDE.md` rules. Findings derived from these references cite the relevant file in `evidence.convention_source` (e.g. `references/avito/migrations.md`).

## Source

Distilled from internal experimental PaaS-review guidelines. Keep these references in sync if the upstream PaaS guidelines change.
