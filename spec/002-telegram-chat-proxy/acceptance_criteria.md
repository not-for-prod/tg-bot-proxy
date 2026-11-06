# Acceptance Criteria: Telegram Chat Proxy MVP

## Status

Derived from the owner-approved solution baseline, including the continuously refilled outbox-publisher change and the
pre-production single-bot design correction.

## Acceptance criteria

1. A first eligible external message persists, creates exactly one chat mapping/topic under normal unambiguous execution, and is copied to that topic.
2. Later external messages reuse the mapped topic.
3. An eligible human message in a mapped topic is copied by the same Telegram bot to the correct private chat without author attribution.
4. Operations for one chat complete strictly by chat offset; ready operations for different chats can be claimed together.
5. Persisting updates and advancing the poll offset is atomic, while duplicate bot/update pairs are ignored.
6. Matching fenced lease ownership is required for poller, inbox processor, and publisher state changes.
7. Retryable errors schedule another attempt with bounded configured delay; permanent or exhausted failures remain queryable in PostgreSQL.
8. A deleted mapped topic cannot receive the next relay in General: the missing root is detected, the topic is replaced
   once, and the external relay is retried once; ambiguous creation is retained as terminal failure.
9. External `/start` queues the configured response. Other external commands and every internal command create no operation.
10. Bot-authored, service, wrong-supergroup, unmapped-topic, and unsupported messages do not relay.
11. All runtime settings are loaded and validated before workers start.
12. The application graph constructs one poller, one inbox processor, and the outbox publisher with concrete repositories and the Telegram gateway.
13. The outbox publisher never exceeds `workerPoolSize`, never claims beyond free capacity or `batchSize`, and refills a slot after its operation finishes while slower unrelated operations remain in flight.

## Success scenarios

- External text and each supported immutable media update become `relay_external_to_internal` operations.
- Internal human topic messages become `relay_internal_to_external` operations.
- `/start` becomes `send_external_message`.
- Empty polling results release ownership without offset movement.
- A successful operation is deleted and advances the expected committed offset.
- A stale completion changes zero durable rows and is reported as stale ownership.
- A fast operation frees a slot that starts another eligible operation before a concurrent slow operation completes.

## Failure scenarios

- Polling HTTP failure leaves the offset unchanged.
- Duplicate updates do not duplicate inbox rows or outbox operations.
- Telegram rate limiting honors `retry_after`.
- Invalid payloads, blocked users, missing permissions, inaccessible destinations, and ambiguous topic creation become terminal failures.
- An expired processing lease can be reclaimed with a higher version; the prior worker cannot complete it.
- A failed head blocks only later operations for the same chat.

## Regression checks

- Existing config loading, PostgreSQL connection, tracing, health checks, metrics, and service lifecycle continue to compile and test.
- No bot token appears in logs, errors, persisted payloads, or test snapshots.
- No generated code is edited without regenerating from its source.

## Required tests

- Config parsing and validation unit tests.
- Telegram gateway request/response and error-classification tests using an HTTP test server.
- Repository integration tests for lease fencing, deduplication, ordering, claiming, completion, retry, failure, and topic mapping.
- Application-service tests for classification, topic-title fallback, operation creation, retry policy, and relay recovery.
- Worker and wiring tests for the single update stream, direction classification, concurrency limits, free-slot refill without a batch barrier, and lifecycle construction.
- The live Telegram integration tests listed in `docs/solution.md` section 17 are operational acceptance checks and may require external credentials/environment.
