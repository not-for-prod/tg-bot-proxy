# Requirements: Telegram Chat Proxy MVP

## Status

Derived from the owner-approved solution baseline, including the continuously refilled outbox-publisher change and the
pre-production single-bot design correction.

## Problem

External Telegram users need anonymous, durable, bidirectional communication with internal team members through one
Telegram bot and one internal supergroup topic per external chat.

## Goals

- Provide durable at-least-once ingestion and delivery.
- Keep per-chat delivery ordered and independent across chats.
- Preserve anonymity and strict eligibility filtering.
- Recover safely from process crashes, expired leases, duplicate updates, missing topics, and retryable Telegram failures.

## Non-goals

The exclusions in `docs/business.md` section 6 are requirements. No domain events or Vibox are required because the application has no cross-aggregate event contract.

## Functional requirements

1. Run one long poller for the configured Telegram bot.
2. Claim polling ownership with a time-limited fenced lease and pass the persisted next update ID to `getUpdates`.
3. Commit received inbox updates, the monotonic next polling offset, and lease release atomically. Empty responses release the lease without changing the offset.
4. Deduplicate inbox updates by bot ID and Telegram update ID.
5. Run one fenced inbox processor for the bot stream and process eligible rows in ascending update ID.
6. Classify external updates as eligible private messages, `/start`, ignored commands, or ignored unsupported/service updates.
7. Classify internal updates as eligible human messages in a mapped topic or ignored bot, command, service, wrong-supergroup, unsupported, or unmapped-topic updates.
8. Select new topic titles using username, display name, then `User <id>`, without renaming existing topics.
9. Create or resolve a chat and atomically enqueue each high-level operation with its next unique chat offset.
10. Support `relay_external_to_internal`, `relay_internal_to_external`, and `send_external_message` operations with the payloads defined in `docs/solution.md`.
11. Claim only ready chat-head operations up to currently free executor capacity and the configured claim-batch size, fence publisher ownership, execute Telegram calls outside transactions, and process different chats concurrently up to the configured worker-pool size. Refill a free executor slot without waiting for unrelated in-flight operations, and stop the scheduled run only after no operation remains in flight and a claim finds no eligible work.
12. On success, delete the operation and advance exactly the expected committed chat offset.
13. Retry retryable failures with configured exponential delays; Telegram `retry_after` overrides the calculated outbox delay.
14. Retain permanent and exhausted inbox/outbox failures with diagnostic details. A failed outbox head blocks later operations only for its chat.
15. Lazily create a topic for an external relay without one. A topic-targeted copy must fail rather than fall back to
    General when its topic root is missing. When a mapped topic is reported missing, create one replacement and retry
    copying once.
16. Treat an ambiguous `createForumTopic` outcome as terminal to avoid automatic duplicate topic creation.
17. Reject stale poller, processor, and publisher completions through lease owner and version checks.
18. Respond to external `/start` with configured text. Ignore other external commands and all internal commands.
19. Never send internal author identity or discussions from unmapped topics to an external user.
20. Delete successfully processed or ignored inbox rows and successfully completed outbox rows.

## Data and contract requirements

- PostgreSQL 15+ stores polling cursors/leases, inbox processor leases, chats, inbox updates, and ordered outbox operations.
- The Goose migration must implement the constraints and indexes defined by `docs/V001__telegram_proxy.sql` and include a reverse-order down migration.
- Telegram Bot API requests use the single configured token and JSON responses; the token must never be logged or stored in database payloads.
- Typed configuration must cover the bot, polling, supergroup ID, `/start` text, inbox settings, outbox settings, and both retry policies. Outbox `batchSize` limits one claim and `workerPoolSize` limits concurrent execution.
- No protobuf contract is introduced because the MVP is worker-driven and exposes no feature API.

## Validation requirements

- Configuration rejects missing bot IDs/tokens, invalid or non-positive durations and batch sizes, invalid retry limits/multipliers, missing supergroup ID, and empty start text.
- Persisted statuses and operations are closed typed sets in Go and constrained in SQL.
- Gateway decoding rejects malformed Telegram responses and invalid operation inputs before a network request.
- Update eligibility checks require the expected chat kind, configured supergroup, topic identifier, human author, and supported immutable message shape.

## Compatibility requirements

- Preserve the existing service entrypoint, health/metrics endpoints, tracing, PostgreSQL connector, and worker abstraction.
- Replace the placeholder initial migration because it has no application schema and the repository has not shipped another migration.
- At-least-once delivery permits a duplicate if Telegram accepts an effect but the process crashes before committing success.
- Private and supergroup updates share Telegram's single bot update stream; per-chat database offsets determine delivery order.

## Open questions

None. The owner confirmed `docs/solution.md` is the complete implementation baseline and that Vibox/domain events are not required.
