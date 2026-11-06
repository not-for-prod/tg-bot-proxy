# Proposal: Telegram Chat Proxy MVP

## Status

Derived from the owner-approved implementation baseline in `docs/solution.md`, including the approved continuously refilled
outbox publisher and the pre-production single-bot design correction. The owner explicitly waived a separate event-storming
artifact because the complete application behavior and recovery model are already defined there.

## Summary

Implement the single-tenant Telegram chat proxy described by `docs/business.md` and `docs/solution.md`: durably poll one
Telegram bot, classify private and internal-supergroup updates from its unified stream, translate eligible updates into
per-chat ordered operations, and relay immutable messages anonymously between external private chats and mapped internal
forum topics.

## Goals

- Persist Telegram updates before advancing polling offsets.
- Deduplicate updates and fence concurrent pollers and processors.
- Maintain one current forum topic per external chat.
- Relay eligible messages in both directions without exposing internal identities.
- Preserve operation order within a chat while allowing different chats to run concurrently.
- Reuse each free outbox executor slot immediately instead of waiting for the slowest operation in a claimed batch.
- Retry safe failures and retain terminal failures for manual investigation.
- Make the feature runnable through existing configuration, worker, PostgreSQL, tracing, metrics, and Fx patterns.

## Non-goals

All exclusions in `docs/business.md` section 6 apply. In particular, the MVP has no domain-event broker, Vibox integration, public API, administrative UI, assignment model, message archive, edit/delete synchronization, or automatic repair interface.

## Behavioral Source

- Product behavior: `docs/business.md`
- Implementation and recovery baseline: `docs/solution.md`
- Configuration example: `docs/config.yaml`
- Initial schema input: `docs/V001__telegram_proxy.sql`
