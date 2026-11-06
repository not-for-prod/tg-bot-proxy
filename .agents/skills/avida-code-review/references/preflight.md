# Preflight Checks

> **Not applicable in a headless runtime (e.g. avida).** Preflight runs the project's lint /
> typecheck / test commands, which need a shell. With no shell, SKILL.md Phase 0b **skips**
> preflight and records "no preflight available" in the run summary. This file documents the
> behaviour for a shell-capable harness only.

Before reviewer agents are dispatched, the orchestrator runs the project's own quality gates. The point is to **filter out issues the project already knows how to find** so reviewer agents don't waste budget on them and developers don't get duplicate alerts.

## Discovery order

Look for preflight commands in this order (first hit wins, do not invent commands):

1. **Explicit project hook** — `make preflight`, `make review-check`, `npm run preflight`, `pnpm preflight`, `yarn preflight`, `bun run preflight`.
2. **Documented in `AGENTS.md` / `CLAUDE.md`** — search for a "preflight", "pre-merge", or "before review" block listing commands.
3. **Convention by stack**:
   - `package.json` `scripts` containing `lint`, `typecheck`, `test`.
   - `pyproject.toml` `[tool.poetry.scripts]` / `Makefile` targets like `lint`, `mypy`, `test`.
   - `go.mod` → `go vet ./...`, `golangci-lint run`, `go test ./...`.
   - `Cargo.toml` → `cargo clippy --no-deps`, `cargo test`.
4. **None found** — skip preflight, note "no preflight available" in the final report.

## Execution rules

- Run each command with a hard timeout (default 5 min). On timeout, mark `skipped`, do not retry.
- Capture **only**: pass/fail status, count of issues (if tool emits one), and the first 50 lines of failure output.
- **Never** paste raw preflight output into reviewer-agent prompts. Reviewers receive a summary, not the firehose.
- If a preflight command fails or reports issues, surface in the final report as a pre-review banner — do **not** down-prioritise reviewer findings because preflight failed; both can be true.

## Tool budget cap

Preflight is allowed at most three commands. If a project lists more (e.g. lint + typecheck + unit + integration + e2e), pick the three that are fastest and most signal-dense (typically lint, typecheck, fast unit). Tell the user what was skipped.

## Output shape

```
{
  "preflight": [
    { "command": "npm run typecheck", "status": "pass" | "fail" | "skipped" | "timeout",
      "issues_count": 0, "summary": "first 50 lines or null" },
    ...
  ]
}
```

The orchestrator prepends a one-line banner to the final report:

> **Preflight:** typecheck ✅ · lint ❌ (12 issues) · unit ✅

## What preflight does NOT replace

Preflight catches the obvious mechanical defects. It does not catch:

- Multi-tenant leaks, missing tenant predicates, auth bypass on a new branch.
- Race conditions, idempotency breaks, lost updates.
- Architectural drift, layering violations.
- Overengineering / simplicity issues.
- Missing tests for important new behaviour.
- Backward-incompatible API changes.

These remain the reviewer agents' job. Preflight is a filter, not a replacement.
