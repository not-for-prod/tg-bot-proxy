# Event-Storming Review

## Status

- Status: Draft | Approved | Superseded
- Owner:
- Reviewed:

## Product Boundary

- Use cases:
- Actors:
- External systems:
- Explicitly excluded behavior:

## Business Flows

For each flow, record:

```text
actor or source
-> command
-> aggregate transition
-> state-change event
-> policy
-> next command
```

Include rejection, timeout, duplicate delivery, ambiguous external outcome, cancellation, and recovery paths.

## Aggregate Candidates

For every candidate root, record:

- concrete business name and one-sentence purpose
- identity and competing-creation key
- immediate invariants
- owned state and explicitly excluded state
- canonical lock identity
- emitted state-change events
- referenced aggregates and acceptable inconsistency window

## Event Routes

For every cross-aggregate route, record:

- source event
- target command and aggregate
- idempotency identity
- ordering assumption
- retry, replay, and poison-message behavior
- correlation and causation propagation

## External Effects

Record the intent, provider, idempotency key, ambiguous-outcome recovery, and result command for every remote side effect.

## Persistence Estimate

List aggregate roots, bounded owned child collections, technical inbox/outbox state, and rebuildable projections. Do not design columns or let an existing table imply an aggregate.

## Existing-System Disposition

Classify existing behavior and components as:

- preserve
- replace
- delete after replacement verification
- unresolved

## Open Decisions

List only decisions that materially change product behavior, aggregate ownership, security, money, or recovery.

## Approval Gate

Approval confirms the actors, commands, events, policies, aggregate candidates, and cross-aggregate routes. It does not approve schema, API contracts, implementation tasks, or destructive migration commands.
