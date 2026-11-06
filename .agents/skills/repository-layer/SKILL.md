---
name: repository-layer
description: Design, implement, review, or refactor Firestarter repository contracts and PostgreSQL adapters under `internal/domain/repository` and `internal/infrastructure/repository`. Use for interface placement, transaction-aware access, query strategy, xo-generated models, domain-facing filters and lock options, converters, batch persistence, and repository tests.
---

# Repository Layer

## Purpose

Use this skill as the single source of truth for repository contracts and PostgreSQL repository implementations. It covers both design questions and implementation tasks such as `implement repo @internal/infrastructure/repository/...`.

Repositories retrieve and persist data. They do not decide business eligibility, hide use-case policy, or expose persistence mechanics upward.

## Allowed Aggregate Methods

Use CRUD-shaped methods and batch variants as the default for aggregate persistence. The goal is to keep business transitions out of repositories, not to force every read model or concurrency primitive into a misleading CRUD name.

Allowed method families:

- `Create`
- `Get`
- `List`
- `Update`
- `Delete`
- `BatchCreate`
- `BatchUpdate`
- `BatchDelete`

Names may include identity selectors for single-object reads, for example `GetByID`, when that matches local style. Collection reads should normally be ordinary `List(ctx, req, opts...)` methods with explicit filter fields such as IDs, statuses, timestamps, or owners in the request. Use `Exists` or `Count` when that is the actual persistence question and avoids loading rows. Do not encode a business action or eligibility decision in the repository method name.

Not allowed for aggregate repositories:

- `MarkAsSuccess`
- `MarkAsFailed`
- `Confirm`
- `VerifyAllowance`
- `Process`
- `Transition`
- `Close`
- `Cancel`
- `Approve`
- `Reject`
- any method that reads like a command or domain transition

Those operations belong in application commands and domain aggregate methods. The repository should receive an already-transitioned aggregate through `Update`/`BatchUpdate`, or return aggregate data through `Get`/`List` so the application/domain layer can decide what transition to perform.

For command workflows, repositories provide the aggregate access patterns the application layer needs:

- single-aggregate reads with domain-facing lock options such as `repository.ForUpdate()`
- collection reads for event/request batches, also accepting lock options when the application command needs them
- single and batch persistence methods for changed aggregate state

Do not force the application layer into per-item command loops because only single-row repository methods exist. If a use case naturally processes a batch, add a collection-shaped repository contract and implementation.

Batch methods must still stay CRUD-shaped. Prefer `List` with filters, `BatchUpdate`, `BatchCreate`, or `BatchDelete` over business names such as `MarkManyAsSuccess`, `VerifyAllowances`, or `ProcessMany`.

For aggregate state changes, treat CRUD naming as a strong interface rule. A conditional or optimistic update may expose its concurrency condition explicitly, but it still receives an already-transitioned aggregate and must not decide the business transition.

## Non-Aggregate Ports

Do not apply aggregate CRUD naming mechanically to other persistence contracts:

- read-model or projection repositories may expose purpose-specific reads, counts, and existence checks
- event or outbox ports may use `Publish` and `PublishBatch` because they append durable domain events rather than transition aggregates
- inbox, queue, or outbox delivery ports may use technical verbs such as `Claim`, `Ack`, or `Dequeue` when they own transport progress
- compare-and-swap or optimistic updates may make the expected version explicit to prevent read-then-write races

These ports still must not decide domain eligibility, mutate an aggregate behind the caller's back, or create lifecycle events that belong to the aggregate.

## Ownership

- `internal/domain/repository` owns interfaces, request/filter types, and domain-facing repository result types.
- `internal/infrastructure/repository/<aggregate-or-lifecycle>` owns PostgreSQL queries, row scanning, xo models, transaction access, and persistence-specific helpers.
- `internal/generated/xo` owns generated table names, column names, DAO structs, aliases, select helpers, and reusable SQL clauses.
- `internal/pkg/convert` owns reusable domain-to-xo and xo-to-domain mapping.

Do not put SQL, xo models, pgx types, transport DTOs, or business policy in domain repository contracts.

## Repository Boundaries And Coordination

Define one focused repository port per aggregate root or cohesive technical
persistence lifecycle. Do not create one feature-wide repository merely
because several tables, workers, and use cases belong to the same product
feature. A repository interface must have one purpose that can be stated in a
short sentence, and its methods must operate on state with the same ownership,
locking boundary, and lifecycle.

Do not infer one repository per table mechanically. Several tables may belong
to one repository when they implement one immediate persistence invariant.
Conversely, tables in one feature require separate ports when they have
independent commands, leases, retries, retention, concurrency, or reasons to
change. Apply `define-aggregate-boundaries` before deciding the split for
persistent business state.

Repository implementations stay short and persistence-focused. A method
normally builds a query, executes it through `i.tr(ctx)`, scans or converts the
result, classifies persistence errors, and returns. Repositories must not
depend on or call sibling repository ports, open an application transaction,
coordinate a use-case workflow, invoke gateways, or decide which repository
operation happens next.

When a use case needs several focused repositories, the application service
owns the sequence and `trm.Manager.Do` transaction and passes the callback
context to every repository call. Do not hide that coordination behind a god
repository method simply to reduce constructor parameters or keep the workflow
in one adapter. A repository method may use a fixed number of SQL statements
only when they persist one proven immediate invariant owned by that port; if
the steps represent independently meaningful repository operations, expose
them separately and coordinate them in application code.

During review, group interface methods by state, lifecycle, lock identity, and
consumer. Split the interface when the groups have different answers. Tests
that embed a broad repository interface merely to fake one method are evidence
that consumers depend on a god repository and the port needs segregation.

Name a technical persistence port after the durable state or lifecycle it
owns, not after the worker or actor currently driving it. Do not introduce
repositories named `Poller`, `Processor`, `Worker`, or `Coordinator`. For
example, a source cursor and fence that control insertion into an inbox belong
to the inbox persistence lifecycle; the polling worker remains a delivery or
use-case role rather than becoming a repository boundary.

## Package Shape

Repository implementation packages usually contain:

- `implementation.go` for shared wiring, interface assertion, constructor, transaction access, and any query builder already used by the package
- one public method per file, named after the method in snake_case
- `query/` SQL files for every raw SQL statement that is clearer than a query builder
- `query/query.go` for `//go:embed` bindings in a dedicated `query` package; keep raw SQL text out of repository Go source files
- method-local row DTOs only for custom projections that xo cannot represent

Use aggregate- or lifecycle-focused repository packages inside the feature. Do
not create either generic persistence packages or one feature-wide repository
package for unrelated state and methods.

## Implementation Wiring

Match the package's existing constructor first. New PostgreSQL repositories should use the transaction-aware pgx wiring already provided by the template:

```go
type Implementation struct {
    db        *pgxpool.Pool
    ctxGetter *txManagerPgxv5.CtxGetter
}

func NewImplementation(db *pgxpool.Pool, ctxGetter *txManagerPgxv5.CtxGetter) *Implementation {
    return &Implementation{
        db:        db,
        ctxGetter: ctxGetter,
    }
}

func (i *Implementation) tr(ctx context.Context) txManagerPgxv5.Tr {
    return i.ctxGetter.DefaultTrOrDB(ctx, i.db)
}
```

Every DB call must go through `i.tr(ctx)`.

The existing Fx PostgreSQL module provides `trm.Manager` to application services and `*txManagerPgxv5.CtxGetter` to repositories. Do not construct a second transaction manager inside a repository. Always use the callback context passed by `trm.Manager.Do` so `i.tr(ctx)` resolves the active transaction.

## Select Options

Repository contracts that need caller-selected locking should expose an option type owned by `internal/domain/repository`, for example `repository.SelectOption` with constructors such as `repository.ForUpdate()` and `repository.SkipLocked()`. Application and domain packages must not import `internal/generated/xo`.

Infrastructure methods accept the domain-facing options and translate them to xo or SQL behavior inside the adapter:

```go
func (i *Implementation) List(ctx context.Context, req repository.ListRequest, opts ...repository.SelectOption) (*repository.ListResponse, error) {
    builder := sq.Select(...).From(...)
    builder = applyFilter(builder, req.Filter)
    builder = req.Pagination.Apply(builder)
    builder = applySelectOptions(builder, opts...)
}
```

Keep the translation helper private to infrastructure. It may call xo helpers internally, but xo types must not appear in the domain interface or application call site. Prefer options over request booleans such as `ForUpdate` so locking stays explicit near the transaction boundary.

## xo Rules

Use xo-generated artifacts for table-shaped queries:

- `xo.Table_<Entity>` for insert, update, delete, and unaliased table references
- `xo.Table_<Entity>_With_Alias` when selected, filtered, joined, grouped, or ordered columns are qualified
- generated field constants for select, update, filter, and ordering columns
- generated model helpers such as `Columns()`, `Values()`, `ToMap()`, and `SelectColumns()`

Do not invent table names, column strings, parallel DAO structs, or table-shaped row DTOs when xo should provide them. If xo output is missing, inspect migrations/schema and run the project generation path instead of hand-writing generated equivalents.

## Migrations

For a feature added after an existing schema migration, create a new forward
migration. Do not fold the feature into an earlier baseline migration unless
the user explicitly asks to rewrite that migration history.

Use private row DTOs only for genuine custom projections: joins, aggregates, computed columns, CTE outputs, or query-specific shapes.

## Query Construction

Use `github.com/Masterminds/squirrel` for ordinary CRUD, readable selects, joins, filters, ordering, pagination, and simple updates.

Treat Squirrel as mandatory for ordinary single-row CRUD. Before handoff, scan repository implementation files for inline SQL passed to `Exec`, `Query`, or `QueryRow`; each occurrence must be an explicitly justified complex-query exception. Do not leave an ordinary `INSERT`, `UPDATE`, `DELETE`, or `SELECT` inline merely because it compiles.

When review reveals this violation in one repository method, audit and correct every matching production repository query in the project unless the user explicitly narrows the scope. Treat the cited method as evidence of a repeated pattern, not as the complete refactoring boundary.

Use embedded raw SQL under `query/` when the query is clearer as SQL. Each
statement must live in its own descriptively named `.sql` file and be bound by
the repository's `query/query.go`. Never declare raw SQL string constants or
pass raw SQL literals directly to `Exec`, `Query`, or `QueryRow` in production
Go files.

Embedded SQL is especially appropriate for:

- large batch writes
- `unnest`-driven inserts, updates, or upserts
- CTE-heavy queries
- PostgreSQL-specific statements that a query builder makes harder to read

Avoid one-query-per-row loops for complex batch writes. For batch aggregate persistence and event/outbox creation, prefer one set-based statement or a small fixed number of set-based statements using arrays, `unnest`, CTEs, or PostgreSQL upsert forms.

## Converters

Put full entity persistence mapping in `internal/pkg/convert`:

- `convert.<Entity>ToXO(...)`
- `convert.<Entity>FromXO(...)`

Keep package-local conversion only for method-owned custom row DTOs or projections, using small helpers such as `(r listRow) toDomain()`.

Do not place domain-to-xo or xo-to-domain entity converters inside repository packages.

## Method Workflow

When implementing or refactoring a repository method:

1. Inspect the domain repository interface and confirm it has one aggregate or technical-lifecycle purpose.
2. Audit sibling methods and split unrelated state, locks, lifecycles, and consumers instead of extending a god repository.
3. Reject or rename the method if it is not CRUD-shaped for an aggregate repository.
4. Inspect the target implementation package.
5. Inspect adjacent repositories for local style.
6. Inspect xo models, aliases, helper clauses, and converters.
7. Choose existing query tooling, parameterized SQL, or a justified builder.
8. Apply domain-facing `opts ...repository.SelectOption` for caller-controlled locking and translate them inside infrastructure.
9. Execute through `i.tr(ctx)`.
10. Convert persistence shapes to domain shapes before returning.
11. Preserve tracing and error conventions already used by the package.
12. Run focused formatting, tests, and lint for the touched package when feasible.

## Batch Method Shape

When implementing batch repository methods:

- accept domain entities or domain-facing request structs, not xo rows or transport payloads
- preserve caller-selected locking on collection reads with `opts ...repository.SelectOption`
- convert each domain aggregate/event through the normal converter path before building query arguments
- use stable array ordering when passing parallel arrays to `unnest`
- return enough information for the application command to detect missing aggregates, duplicates, or failed optimistic expectations when that matters to the use case
- keep business decisions out of SQL `WHERE` clauses unless they are pure identity, ownership, or persistence predicates already decided by the application/domain layer

Good repository responsibilities:

- `List(ctx, req, repository.ForUpdate())`
- `BatchUpdate(ctx, aggregates)`
- `events.PublishBatch(ctx, domainEvents)` for a distinct durable-event port

Bad repository responsibilities:

- `ConfirmEligiblePayments(ctx, ids)` that decides payment eligibility and creates lifecycle events internally
- `MarkManyAsSuccess(ctx, ids)` that hides a domain transition behind persistence
- `VerifyAllowances(ctx, payments)` that mixes an application decision into the repository
- `ProcessEvents(ctx, payloads)` that parses worker payloads and mutates aggregate state
- looping over `Update(ctx, aggregate)` for every item inside `BatchUpdate` when a set-based update is straightforward

## Method Shape

Separate logical phases inside repository methods with blank lines. In
particular, leave a blank line after tracing setup, after query construction,
after preparing local request, result, or scan-destination values before the
database call, after database error handling, after deferred row cleanup, and
between result validation and the successful return. Keep statements from the
same small phase together, such as `prospan.Start` with its matching
`defer span.End()`. Do not compress tracing, preparation, execution,
validation, and return into an unbroken block.

When review identifies a missing phase separator in one repository method,
audit every production repository file and correct all equivalent boundaries;
do not limit the formatting pass to the cited method.

Typical traced method boundary:

```go
func (i *Implementation) Get(ctx context.Context, id entity.ID, opts ...repository.SelectOption) (*entity.Entity, error) {
    ctx, span := prospan.Start(ctx)
    defer span.End()

    query, args, err := buildGetQuery(id, opts...)
    if err != nil {
        return nil, span.Err(errors.Join(&proterror.Internal{}, err))
    }

    var row xo.Entity
    if err = i.tr(ctx).Get(ctx, &row, query, args...); err != nil {
        return nil, span.Err(proterrors.FromPG(err))
    }

    return convert.EntityFromXO(&row), nil
}
```

`buildGetQuery` is an infrastructure helper that uses the package's chosen query mechanism and translates repository options. Match local package style over forcing this exact shape.

## Testing

Use focused unit tests for pure filters and converters, and PostgreSQL-backed tests when correctness depends on SQL, locking, constraints, pagination, or batch behavior.

Repository methods used by recurring reconciliation workers must not repeatedly expose only the first fixed `LIMIT`. Accept a stable keyset cursor, apply a deterministic `ORDER BY`, and return rows strictly after that cursor so the application can visit the whole eligible set without queue-head starvation.

A recurring scan must either exhaust all keyset pages in the current run or durably persist its continuation cursor. A fixed maximum page count followed by a successful return silently starves rows beyond the cap.

Durable execution claims need an ownership token in addition to a timestamp. Reclaiming a stale row must replace the token, and complete/release/ambiguous transitions must compare that token so an older worker cannot finish a newer lease.

Provider pagination cursors are opaque and may cycle through more than one value. Track every cursor seen during a scan and fail on repetition. A page safety limit is safe only when the next cursor is durably persisted and the following run resumes from it; restarting from the first page after hitting the cap can permanently starve the tail even when the worker returns an explicit error.

Cover the changed contract's relevant cases:

- domain-to-xo and xo-to-domain mapping
- not-found, duplicate, and constraint error classification
- every filter and stable pagination order
- translation of `repository.ForUpdate()` and `repository.SkipLocked()`
- transaction context selection through `i.tr(ctx)`
- batch input/output cardinality, missing IDs, duplicates, and stable parallel-array ordering
- conditional or optimistic updates under stale versions

Do not mock away SQL behavior that the method's correctness depends on. Keep integration tests deterministic and use the repository's local infrastructure path rather than a developer-owned database.

## Boundary Smells

Pause and re-check the design if repository code:

- exposes a feature-wide interface spanning unrelated aggregates, queues, leases, cursors, or delivery lifecycles
- needs several independent purposes to explain the repository in one sentence
- is injected into consumers that use only a small subset of its methods
- causes tests to embed the full repository interface merely to fake one method
- calls another repository, starts `trm.Manager.Do`, invokes a gateway, or coordinates the order of persistence operations
- contains long application-style branching or several independently meaningful persistence phases
- adds a method whose name is not CRUD-shaped for an aggregate repository
- returns `bool` decisions such as `HasCompleted...` that hide use-case policy
- hardcodes table or column names that xo provides
- bypasses `i.tr(ctx)`
- adds `ForUpdate` request booleans instead of select options
- exposes xo, pgx, SQL fragments, or row DTOs through domain interfaces
- implements business eligibility instead of returning data for application/domain code to evaluate
- creates domain lifecycle events or changes lifecycle state without receiving already-transitioned aggregates/events from the application layer
- contains verbs such as `Mark`, `Confirm`, `Verify`, `Process`, `Approve`, `Reject`, `Close`, or `Cancel` in aggregate repository method names
- implements batch methods as unbounded one-query-per-item loops where arrays, `unnest`, or CTEs would fit the local PostgreSQL style
- contains a raw SQL literal or constant in a production `.go` file instead of an embedded `query/*.sql` file
