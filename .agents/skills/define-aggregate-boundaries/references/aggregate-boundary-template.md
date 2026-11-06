# Aggregate Boundary Template

Use this template as the aggregate-boundary section of `domain-model.md`. Write in business language and delete instructional text from the completed artifact.

## Scenario

- Actor or source:
- Command or observed fact:
- Expected business outcome:
- Hard consistency requirement:
- Acceptable temporary inconsistency window:

Describe the complete behavior in no more than three short sentences.

## Candidate Aggregate Roots

| Root | One-sentence purpose | Identity / creation key | Owned state | Explicitly excluded state | Immediate invariants | Canonical lock and protected rows | Bounded mutable/per-command write set | Write repository |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |

For every existing-root command, name the repository call or option that produces `SELECT ... FOR UPDATE`.

## Relationship Decisions

| Source | Related object | Owned or referenced | Immediate invariant, if owned | Why eventual consistency is safe, if referenced | Maximum inconsistency window / effective time | Source event | Target command | Idempotency identity |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |

Do not use “same transaction for convenience” or “there is a foreign key” as a boundary reason.

## Command Transactions

For each command, record:

| Command | Root | Root lock | Owned rows changed | State-change events | Other roots reached later |
| --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |

Each row must name one root only. Record technical inbox/outbox rows separately from business roots.

## Vocabulary Check

For every root:

- Is the name a concrete noun used by the business?
- Can a domain owner understand its purpose without implementation context?
- Does the purpose fit in one short sentence without combining independent responsibilities?
- Do commands use exact business verbs?
- Is one term used consistently for one concept?
- Can a new engineer read the event route as the business story?

List rejected vague names and the concrete replacements chosen.

## Boundary Challenges

Record alternatives considered:

| Proposed combined state | Smallest immediate invariant | Can it be eventually consistent? | Split or keep | Evidence |
| --- | --- | --- | --- | --- |
|  |  |  |  |  |

## Exceptions

For every requested exception, record the exact rule, unavoidable invariant, rejected event/reservation/compensation alternatives, lock and recovery consequences, owner, and review date. Write `None` when there are no exceptions.

## Approval

- Owner:
- Decision:
- Date:
- Open questions:
