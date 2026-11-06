---
name: define-aggregate-boundaries
description: Define, review, or refactor small transactional aggregate roots with simple business semantics. Use before planning or implementing any persistent stateful feature when deciding consistency boundaries, ownership, repositories, transactions, SELECT FOR UPDATE locking, entity relationships, eventual consistency, or aggregate names and purposes; also use when an existing aggregate is broad, vague, difficult to explain, or updated alongside other business objects.
---

# Define Aggregate Boundaries

Use this skill before `design-event-driven-aggregates`, `domain-entity`, `application-layer`, and `repository-layer`. It decides what each aggregate is; the later skills decide its transitions, events, orchestration, and persistence implementation.

The objective is production code with the smallest safe transaction and the clearest possible business story. Do not infer boundaries from tables, foreign keys, provider payloads, API messages, existing packages, or code that happens to share a service.

## Definition

An aggregate is the smallest bounded set of persistent business state that must satisfy an invariant immediately when one database transaction commits.

Its root:

- is the only entry point for changing that state;
- owns every entity and value object that participates in the same immediate invariant;
- is loaded and locked inside the mutation transaction;
- is saved as one consistency unit through one write repository;
- refers to other aggregate roots by typed identity, not mutable object ownership.

An aggregate may span several rows or tables. It is not automatically one table. It must nevertheless remain bounded, cheap to load, and safe to lock under expected concurrency.

## Non-Negotiable Rules

1. Choose the smallest boundary that can enforce the immediate invariant.
2. For an existing aggregate, begin the transaction and load its canonical root row with `SELECT ... FOR UPDATE`, normally through an explicit repository option such as `repository.ForUpdate()`. Lock this same root before every owned-child write.
3. Let one command and one transaction lock and mutate at most one aggregate root and its owned state.
4. Keep application code from writing an owned entity through a separate repository. The aggregate repository hides its owned persistence.
5. Put two objects in different aggregates when their relationship can be temporarily inconsistent and restored through durable eventual consistency.
6. Coordinate different aggregates with state-change events and idempotent consumers by applying `design-event-driven-aggregates` next.
7. Do not put a remote call, broker wait, or another aggregate's lock inside the aggregate transaction.
8. Do not treat a foreign key, shared creation request, shared screen, provider response, or reporting query as proof of aggregate ownership.
9. Keep the transactional working set bounded. Do not require loading or locking an unbounded history or collection to perform a command. A command may append only a small, bounded number of immutable root-owned records.
10. Require an explicit owner-approved architecture exception for a multi-root transaction, an unlocked mutable root, or another departure from these rules.

For creation, there may be no row to lock yet. Serialize competing creation with a database uniqueness or locking strategy for the real business identity, then continue to use `FOR UPDATE` for later mutations. Do not use this edge case to weaken locking for existing roots.

## The Boundary Test

For each proposed root, answer these questions in business language:

1. What invariant must be true immediately at commit?
2. Which exact state must be inspected and changed to enforce it?
3. Which rows must be locked together for concurrent commands to remain correct?
4. Can any relationship be temporarily inconsistent without violating money, authorization, inventory, or another hard invariant?
5. Can a durable event and idempotent command repair or advance that relationship?
6. Does a proposed child have its own identity, lifecycle, commands, concurrency, or retention policy?
7. Can the owned collection grow without a hard bound?
8. Would different parts change for different business reasons or at very different rates?

Keep state together only when the first three answers prove an immediate invariant and the boundary remains bounded. Split it when eventual consistency is safe, the state has independent lifecycle or concurrency, or ownership would create an unbounded lock.

When uncertain, prefer two small aggregates and make the consistency requirement explicit. Do not create one large aggregate merely to avoid designing the event between them.

## Owned State Versus Another Aggregate

Treat a related entity as owned state only when all of these are true:

- it has no meaningful write lifecycle outside the root;
- callers never address or mutate it independently;
- an invariant requires it to commit atomically with the root;
- it is always reached and protected through the root lock;
- its mutable working set and per-command write set are operationally bounded.

Otherwise make it another aggregate, a value copied into the root, or a rebuildable read projection.

Examples of non-ownership:

- an `Order` references a `CustomerID`; it does not own the customer;
- a `WestWalletWebhook` references the provider order identity; it does not own the `WestWalletOrder`;
- a `WestWalletOrder` can cause a `CustomerBalance` command, but it does not own the balance;
- a reporting relationship or SQL join does not merge aggregate boundaries.

A business `WestWalletWebhook` aggregate represents an authenticated provider fact with business identity or lifecycle. A transport inbox row used only for deduplication is technical transaction state, not another aggregate.

An append-only journal may grow historically without making the transaction unbounded. It can be root-owned when each command appends a small bounded number of immutable entries, every append first locks the canonical root row, no command scans or locks the history, and entries cannot be corrected or transitioned independently. This supports an atomic balance-and-journal invariant without loading the full ledger. If the journal is independently commanded, corrected, or treated as a separately locked authority, redesign and name that aggregate explicitly.

## Transaction Shape

For an existing root, require this shape:

```text
begin transaction
load one root by identity using SELECT FOR UPDATE
invoke one intention-revealing root command
save the root and its owned state
append the root's state-change events to the transactional outbox
commit
```

Every path that writes an owned row must first lock the same canonical root row; locking a child row alone does not serialize the aggregate. The application service coordinates this unit of work; it does not assign lifecycle fields or update several repositories and call the result an aggregate transaction.

If the transaction needs a second root to decide or complete the command, first challenge the model:

- copy the stable fact needed by the first root;
- react to the other root's event;
- issue a later idempotent command;
- model reservation, intent, or compensation explicitly.

Use a multi-root transaction only when the owner approves a documented invariant that truly cannot tolerate eventual consistency.

## Human-Simple Semantics

An aggregate must be easy for a production engineer and a domain owner to understand.

Apply these rules:

- Name the root with one concrete business noun used by the domain, such as `WestWalletWebhook`, `WestWalletOrder`, or `CustomerBalance`.
- State its purpose in one short sentence: “A CustomerBalance protects and changes one customer's available funds.”
- If the purpose needs independent clauses joined by “and” or “or”, test whether it contains several aggregates.
- Name commands with exact business verbs such as `Receive`, `Confirm`, `Credit`, `Reserve`, or `Cancel`.
- Make packages, repositories, commands, and events use the same noun for the same concept.
- Prefer code that reads as the business story over abstract frameworks or speculative reuse.
- Add only state and abstractions required by a current invariant or behavior.
- When a payable action earns a client fee and incurs provider cost, keep the
  fee, cost, and charge decision on that action root. A parent workflow does
  not own action pricing merely because it starts the action, and a balance
  root owns reservation mechanics rather than the action's pricing policy.

Reject vague names such as `Item`, `Record`, `Data`, `Context`, `Resource`, `Manager`, `Processor`, or `Coordinator` when a concrete domain term exists. Challenge broad names such as `Account`, `Transaction`, `Order`, or `Status` until their exact business meaning is obvious; keep them only when they are genuinely the domain's term.

Do not invent synonyms. If the business says `WestWalletWebhook`, do not alternate between notification, callback, evidence, receipt, and message for the same aggregate.

## Required Workflow

### 1. Discover mutations and invariants

Inspect commands, transactions, repositories, `FOR UPDATE` usage, schemas, entity relationships, events, consumers, and concurrency tests. List every current write path and the business invariant it claims to protect.

### 2. Propose the smallest roots

For each candidate root, define:

- exact business name and one-sentence purpose;
- identity and creation key;
- owned and explicitly excluded state;
- immediate invariants;
- lock identity and rows protected by the root lock;
- write repository and bounded-size assumptions;
- related aggregates and why their consistency may be eventual.

Use [the aggregate boundary template](references/aggregate-boundary-template.md) to create or update the aggregate-boundary section of `domain-model.md` before production implementation.

### 3. Prove every relationship

For every link between business objects, record either:

- **owned and immediate**: the invariant, root lock, and bounded state that justify one aggregate; or
- **referenced and eventual**: the source event, target command, idempotency identity, and acceptable inconsistency window.

An unexplained relationship is not approval to merge roots.

### 4. Simplify vocabulary and purpose

Read the proposed names and purposes as a short business narrative. Rename or split anything that requires implementation knowledge, several sentences, or a glossary of invented abstractions.

### 5. Obtain approval and route forward

For a non-trivial feature, obtain explicit owner approval for the boundary map before schema or production code changes. Then apply `design-event-driven-aggregates` to define transitions and cross-root event routes, followed by the relevant layer skills.

## Review Heuristics

Flag:

- a command or transaction that writes multiple root repositories;
- an existing mutable root loaded without `FOR UPDATE`;
- a service that locks child rows without loading the root;
- direct field updates that bypass the root;
- aggregates named after technical records or vague abstractions;
- a purpose that combines unrelated responsibilities;
- a child with independent commands, lifecycle, or concurrency;
- an unbounded collection inside the lock boundary;
- cross-root object graphs instead of typed IDs;
- remote calls or message waits while holding the root lock;
- large aggregates justified only by joins, foreign keys, or synchronous convenience;
- eventual consistency rejected without naming the immediate invariant that forbids it.

For each finding, report the concrete write path, violated invariant or rule, concurrency/failure consequence, and smallest simpler boundary.

## Exceptions and Governance

An exception must record:

- the exact roots and operation involved;
- the invariant that must be immediately consistent;
- why an event, reservation, compensation, or boundary redesign is insufficient;
- lock order, contention, deadlock, retry, and recovery consequences;
- owner, compensating controls, and review/removal condition.

Legacy shape, implementation cost, deadlines, fewer files, and easier synchronous code are not valid implicit exceptions. Do not modify this skill or governing architecture rules inside the feature change they govern.
