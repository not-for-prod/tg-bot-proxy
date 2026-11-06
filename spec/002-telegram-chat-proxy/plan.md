# Implementation Plan: Telegram Chat Proxy MVP

## Status

Implemented and verified, including the approved continuously refilled outbox-publisher change and the pre-production
single-bot design correction. The patch updates this plan and its execution record in place because it replaces a design
choice rather than adding a feature.

## Implementation phases

1. Runtime contracts and schema
2. Domain-facing ports and core values
3. PostgreSQL and Telegram adapters
4. Application workflows
5. Delivery workers and composition
6. Verification and operational documentation

## Tasks

## Phase 1: Runtime contracts and schema

### task-1.1 Add validated Telegram runtime configuration

Goal: Bind all settings in `docs/config.yaml` to typed startup configuration.

Files:
- `config/config.go`
- `config/config.yaml`
- `config/config_test.go`

Acceptance:
- Acceptance criteria 11.

Verification:
- `go test ./config`

### task-1.2 Replace the placeholder migration

Goal: Install the documented proxy persistence model with reversible Goose SQL.

Files:
- `tools/migrations/20251220070647_init.sql`
- `internal/generated/xo/**` after generation

Acceptance:
- Acceptance criteria 4 through 7.

Verification:
- `make migrations-up`
- `make xo`

## Phase 2: Domain-facing contracts

### task-2.1 Add core types and gateway/repository ports

Goal: Define small typed models and inward-facing interfaces without provider or SQL leakage.

Files:
- `internal/domain/entity/**`
- `internal/domain/repository/**`
- `internal/domain/gateway/**`

Acceptance:
- Acceptance criteria 1 through 10.

Verification:
- `go test ./internal/domain/...`

## Phase 3: Infrastructure adapters

### task-3.1 Implement PostgreSQL durability and ordering

Goal: Implement polling, inbox, chat, and outbox behavior with transactions, row locks, skips, and fencing.

Files:
- `internal/infrastructure/repository/telegram-proxy-repository/**`

Acceptance:
- Acceptance criteria 4 through 7.

Verification:
- `go test ./internal/infrastructure/repository/...`

### task-3.2 Implement the Telegram Bot API gateway

Goal: Implement the four required Telegram calls and deterministic outcome classification.

Files:
- `internal/infrastructure/gateway/telegram/**`

Acceptance:
- Acceptance criteria 1 through 3 and 7 through 10.

Verification:
- `go test ./internal/infrastructure/gateway/...`

## Phase 4: Application workflows

### task-4.1 Implement polling and inbox processing services

Goal: Persist updates safely and convert eligible updates into ordered operations.

Files:
- `internal/application/service/chat/**`

Acceptance:
- Acceptance criteria 1 through 6 and 9 through 10.

Verification:
- `go test ./internal/application/service/...`

### task-4.2 Implement publishing and relay recovery

Goal: Claim ordered operations, execute effects, and complete, retry, fail, or replace topics safely.

Files:
- `internal/application/service/chat/**`

Acceptance:
- Acceptance criteria 1 through 8.

Verification:
- `go test ./internal/application/service/...`

## Phase 5: Delivery and composition

### task-5.1 Add poller, inbox, and outbox workers

Goal: Schedule the single bot update stream and bounded concurrent outbox execution. Drain ready outbox work through global executor slots, refilling each free slot without a batch barrier.

Files:
- `internal/delivery/worker/**`

Acceptance:
- Acceptance criteria 4, 12, and 13.

Verification:
- `go test ./internal/delivery/worker/...`

### task-5.1a Replace batch-and-wait outbox dispatch

Goal: Pass free executor capacity into operation claiming, cap each claim by configuration, and continuously refill bounded executor slots until no ready work remains.

Files:
- `internal/domain/service/telegram_proxy.go`
- `internal/application/service/chat/claim_operations.go`
- `internal/application/service/chat/claim_operations_test.go`
- `internal/delivery/worker/chat/**`

Acceptance:
- Acceptance criteria 4, 6, and 13.

Verification:
- `go test ./internal/application/service/chat ./internal/delivery/worker/chat`

### task-5.2 Wire the complete Fx graph

Goal: Construct concrete gateways, repositories, services, and three workers through the existing entrypoint.

Files:
- `cmd/main.go`

Acceptance:
- Acceptance criterion 12.

Verification:
- `go test ./cmd ./...`

## Phase 6: Completion

### task-6.1 Verify and document operation

Goal: Run formatting, generation, focused and broad tests, lint, and document remaining live checks.

Files:
- `README.md`
- generated files required by changed sources

Acceptance:
- All acceptance criteria.

Verification:
- `make fmt`
- `go test ./...`
- `make linter`

## Checkpoints

- No owner checkpoint is required before routine phases because the complete observable behavior is approved and the active request authorizes implementation.
- Return for approval only if implementation requires a new external dependency, changes observable behavior, changes the one-chat ordering model, introduces a public API, or needs a destructive database reset.

## Verification commands

Run task-local commands during implementation, then `make migrations-up`, `make xo`, `go test ./...`, and `make linter`. Run the live Telegram tests from `docs/solution.md` section 17 only with explicitly supplied test credentials and supergroup access.

## Execution record

- All six phases were implemented without changing the approved observable behavior.
- The migration was applied, rolled back, and reapplied against the isolated `tg_bot_proxy_test` PostgreSQL database.
- xo output was regenerated from that schema.
- Focused PostgreSQL tests verified polling fencing/deduplication and chat-head operation completion.
- The outbox claim contract accepts free executor capacity and caps each database claim by the configured batch and pool sizes.
- The outbox publisher refills a completed executor slot before unrelated slower operations finish; a focused race test verifies refill timing and peak concurrency.
- `go test ./...` and `make linter` passed.
- Live Telegram acceptance checks remain environment-dependent and were not run because no test bot credentials or supergroup were supplied.
- The pre-production design patch removed the bot-role split from configuration, polling, inbox processing, and gateway
  token selection. Direction is classified from private-chat versus configured-supergroup context.
- `go test ./...` and `golangci-lint --config .golangci.yaml run ./...` passed after the single-bot patch.
- Deleted-topic delivery was hardened after live behavior showed that Telegram can accept a stale `message_thread_id` and
  route the copy to General. Topic copies now require the topic root reply target so the existing replacement flow runs.
