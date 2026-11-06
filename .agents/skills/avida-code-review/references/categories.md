# Production-Readiness Categories

Every finding carries exactly one category. Use the most specific applicable category. If two fit, pick the one that drives the priority.

| category | When to use |
|---|---|
| `correctness` | Wrong behaviour, missed branch, invalid state transition, off-by-one, wrong default. |
| `business_logic` | Broken domain rule or incomplete product behaviour. |
| `backward_compatibility` | Broken API / RPC contract, message schema, public type, config default, migration. |
| `security` | Exploitable security issue or sensitive-data handling problem. |
| `tenant_isolation` | Cross-tenant data leak, missing tenant predicate, wrong tenant context. |
| `data_integrity` | Corruption, duplication, orphaning, lost update, unsafe transaction scope, double-write inconsistency. |
| `performance` | N+1, unbounded scan, missing pagination/index where production scale matters, hot-path allocation. |
| `reliability` | Race condition, non-idempotent retry, goroutine/task leak, unsafe failure mode leaving inconsistent state. |
| `observability` | Silent decision/branch where project policy requires debug logs, metrics, or trace context. |
| `test_coverage` | Important changed behaviour, edge case, or regression path lacks tests. |
| `architecture` | Layering, DI, event bus, builder/repository pattern, or project-pattern violation. |
| `maintainability` | Major code smell making ownership materially harder (god module, hidden coupling, naming traps). |
| `simplicity` | Unnecessary complexity, overengineering, excessive branching/indirection, duplicated decision logic, solution larger than the problem. |
| `dead_code` | Newly introduced unreachable or unused code that hides incomplete implementation. |
| `design_mismatch` | Implementation contradicts spec, PR intent, docs, or surrounding design. |

## Rules

- **Simplicity is first-class** — report overcomplicated code even when behaviour currently works, if the solution is not simple-and-sufficient for the task.
- **Style and taste are out of scope** — formatting, single-quote vs double-quote, personal preferences, broad cleanup unrelated to the diff, and speculative risks without code evidence must NOT be reported.
- **One category per finding** — if multiple apply, choose the one that explains why it's release-relevant.
- **Category drives priority intuition** — `security` / `tenant_isolation` / `data_integrity` skew P0/P1; `simplicity` / `maintainability` stay ≤ P1 and exceed it **only** via the maintainability cross-walk in `references/rubric.md` (an unjustified presumptive blocker).

## Ambition note

`simplicity` and `maintainability` are judged against the ambition directive (rule 0) and the
eight-rule standard in `references/maintainability.md`. Severity for these categories follows the
single maintainability cross-walk in `references/rubric.md`: they stay ≤ P1 unless the diff hits an
**unjustified presumptive blocker**, and reach P0 only when the issue also crosses correctness /
security / data-integrity. There is no separate "escalate above P1" rule here.
