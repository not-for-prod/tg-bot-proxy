# Architecture Constraints: Telegram Chat Proxy MVP

## Status

Derived from the owner-approved solution baseline, including the continuously refilled outbox-publisher change and the
pre-production single-bot design correction.

## Layer ownership

- `config` owns typed startup configuration and validation.
- `internal/domain/entity` owns the small `Chat` business model and closed operation/status/value types where they express business meaning.
- `internal/domain/repository` and `internal/domain/gateway` own inward-facing persistence and Telegram contracts.
- `internal/application/service` owns transactional use cases: polling persistence, inbox conversion, operation claiming/completion, and relay orchestration.
- `internal/infrastructure/repository` owns pgx SQL and database row mapping.
- `internal/infrastructure/gateway/telegram` owns Telegram HTTP/JSON transport and Telegram-specific errors.
- `internal/delivery/worker` owns polling loops, scheduled dispatch, and bounded concurrent execution.
- `cmd` owns Fx construction only.

## API and protobuf constraints

No public feature API or protobuf schema is needed. Telegram Bot API is an infrastructure gateway, not an application delivery API.

## Domain constraints

- Prefer a small `Chat` model with intention-revealing offset/topic behavior; do not model inbox, outbox, or leases as domain events or business aggregates.
- Use typed operation and status values instead of raw strings outside adapter serialization.
- Do not introduce Vibox, a broker, or domain-event publication.
- Keep Telegram provider DTOs and JSON out of domain entities.

## Application service constraints

- Telegram network calls must occur outside database transactions.
- Infer inbound direction from chat context in the single update stream: private chat for external input, configured
  supergroup topic for internal input. Do not reintroduce a bot-role selector.
- Application services own transaction boundaries and stale-lease decisions.
- One transactional mutation locks at most one chat root and its ordered operation state.
- Do not hide retry loops in the gateway; durable retry scheduling belongs to the application workflow.
- Operation claiming accepts requested free executor capacity and caps one database claim by the configured outbox batch and pool sizes.

## Delivery worker constraints

- The outbox publisher uses a global bounded pool; workers are not permanently assigned to chats.
- Refill a free executor slot as soon as its operation finishes instead of waiting for all concurrent operations.
- Claim only work that can be dispatched immediately; do not create a buffered backlog of leased operations.
- End a scheduled publisher run only after no operation remains in flight and a claim returns no eligible work.

## Repository constraints

- Use pgx and the existing Avito transaction manager/context getter.
- Existing chat mutations lock the canonical `chats` row with `FOR UPDATE`.
- Use conditional writes with owner/version fencing and return a typed stale-lease error when zero rows change.
- Use `FOR UPDATE SKIP LOCKED` only for short claim transactions.
- Keep queries explicit and human-readable; generated xo models may be used after schema generation, but hand-written behavior queries remain in repositories.

## Gateway constraints

- Use the repository-mandated Resty v2 client with config-owned base URL, timeout, and debug settings; tests replace Resty's transport.
- Implement only `getUpdates`, `createForumTopic`, `copyMessage`, and `sendMessage` required by the MVP.
- Classify Telegram rate limits, temporary server failures, transport failures, permanent API failures, missing topics, and ambiguous topic creation distinctly.
- Never include bot tokens in errors, tracing attributes, or logs.

## Migration constraints

- Replace the placeholder initial Goose migration with the approved PostgreSQL schema and a reverse-order `DROP TABLE IF EXISTS` down section.
- Do not wrap Goose statements in an extra `BEGIN`/`COMMIT`.
- Preserve constraints, partial indexes, uniqueness, and comments that enforce the documented durability model.

## Error handling constraints

- Apply `proterror` at error origins according to repository conventions; preserve `errors.Is/As` classification for stale leases and Telegram outcomes.
- Store concise operational failure data without secrets or entire HTTP requests.
- Never turn an ambiguous topic-creation outcome into an automatic retry.

## Tracing and observability constraints

- Context-aware application, repository, gateway, and worker operations follow `prospan` lifecycle conventions.
- Record bot/chat/update/operation identifiers only when safe; never record tokens or full message content.
- Worker failures remain visible through tracing/logging while durable retry/failure state remains authoritative.

## Generation constraints

- Run migrations against local PostgreSQL and `make xo` after the schema source changes.
- Run the narrowest `go generate` commands for repository/gateway mocks if generator tools are available.
- Run formatting, focused tests, `go test ./...`, and the repository linter after implementation.
