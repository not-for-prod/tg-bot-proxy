# Avito Toolchain — Codegen, Format, Lint

**Activates when**: Avito stack detected (`app.toml`, `brief/`, `auth.toml`, `nfr.toml`, `db/changelog/`).

**Used by**: `code-reviewer`, `code-simplifier`, `golang-pro`, `fullstack-developer`.

These are project conventions for **how to invoke** toolchain commands. The reviewer doesn't run them — it judges whether the diff respects them (via CI configs, scripts, hooks, README) and whether suggestions in the review align with what the project actually runs.

## CRITICAL: do not invent custom invocations

Avito services run on a wrapped toolchain. Reviewers must not propose raw `gofmt` / `go generate` / per-tool codegen calls when the project ships `avito` CLI shortcuts. Project settings (lint rules, codegen sources, formatter flags) are baked into the wrapper — bypassing it produces divergent output.

| Bad suggestion in a review | Correct suggestion |
|---|---|
| "Run `gofmt -s ./...`" | "Run `avito fmt`" (or `golangci-lint fmt` if avito CLI absent). |
| "Run `golangci-lint run ./...`" | "Run `avito lint`" — project linters config is loaded automatically. |
| "Add a `go generate` step for new RPC client" | "Run `avito codegen`" — regenerates **all** project codegen artefacts (Brief, RPC clients, mocks). |
| "Use `gofmt -w`" on a CI script | `gofmt` formatting differs slightly from `golangci-lint fmt` — never mix. Use the same formatter the project enforces. |

## Codegen discipline

`avito codegen` is the **single entrypoint** for code generation. Running individual generators (`oapi-codegen`, `mockgen`, `protoc`, `briefgen`) bypasses project orchestration and is a finding when:

- A new `go:generate` directive is added in the diff with a one-off generator the project's `avito codegen` doesn't know about → P2 (out-of-band codegen).
- A CI step or `Makefile` target invokes a specific generator instead of `avito codegen` → P2 (drift risk between local and CI).

Exception: when the project's `AGENTS.md` / `CLAUDE.md` documents a generator outside `avito codegen` as an intentional escape hatch, accept it.

## Formatter / linter discipline

| Tool | Avito-wrapped form | Raw fallback (only if wrapper unavailable) |
|---|---|---|
| Formatter | `avito fmt` | `golangci-lint fmt` |
| Linter | `avito lint` | `golangci-lint run` |
| Tests | `avito test` | `go test ./...` |

**Never** mix `gofmt` and `golangci-lint fmt` in the same project — their output diverges (import grouping, simplification). If both appear in CI/scripts in the diff → P2.

## Agent-friendly avito CLI

The internal avito CLI has an agent-oriented mode (compact output, no progress bars, token-economical). Where the reviewer suggests a manual repro command, prefer the agent-friendly variant if mentioned in `AGENTS.md`. Don't fabricate flags — check the project docs.

## What NOT to report

- Lack of explicit `gofmt` step when `avito fmt` is wired in CI — not a finding.
- Missing `go generate` directives in files when `avito codegen` regenerates them centrally — not a finding.
- Style flags / line-length values — project linters config (loaded by `avito lint`) is the source of truth.

## Quick checklist

1. Diff adds raw `gofmt` / `golangci-lint run` invocations alongside existing `avito fmt`/`avito lint` — flag drift.
2. New generator wired via `go:generate` without entry in `avito codegen` config — flag (P2).
3. CI step regenerates code via per-tool invocation instead of `avito codegen` — flag.
4. Review suggestion advises raw toolchain when the project clearly uses the wrapper — correct it before emitting.
