---
name: go-code-style
description: Resolve Firestarter-specific Go implementation-style questions using the repository lint contract and the Uber Go Style Guide. Use for focused style reviews or ambiguous choices involving interfaces, initialization, concurrency, map or slice ownership, imports, or table-driven tests; use `naming` for identifier work and `function-decomposition` for function shape.
---

# Go Code Style

## Source

Follow the official Uber Go Style Guide:

- https://github.com/uber-go/guide/blob/master/style.md

Use this skill as the local checklist. If a question is subtle or not covered here, consult the official guide and prefer local repository conventions when they are stricter.

## Baseline

- Always run `gofmt` on edited Go files.
- Prefer `goimports` when import grouping or unused imports are involved.
- Keep code simple, explicit, and idiomatic.
- Match the surrounding package style unless it conflicts with this guide or project skills.
- Write the smallest code shape that fully solves the task.
- Prefer direct readability over cleverness, speculative extension points, or imaginary future performance.
- Do not add argument or field validation as a general code-style reflex. Validation placement is owned by the `field-validation` skill: validate field shape at first occurrence only, and do not repeat it through every layer.

## Interfaces

- Do not use pointers to interfaces.
- Accept interfaces and return concrete types when that keeps ownership clear.
- Verify interface compliance when a type is meant to satisfy an important contract:

```go
var _ repository.Outbox = (*Implementation)(nil)
```

## Errors

- Handle each error once.
- Wrap or classify errors at the lowest meaningful layer according to project error skills.
- Do not panic for normal error paths.
- Error strings should be lower-case and should not end with punctuation.
- Prefer specific error context over generic messages.

## Data Boundaries

- Copy slices, maps, and mutable byte-like values when crossing ownership boundaries.
- Do not expose internal mutable state from entities, DTOs, caches, or config.
- Use field tags only on structs that are actually marshaled, scanned, or bound by a tool using those tags.

## Concurrency

- Use `defer` for cleanup such as unlocks and closing resources.
- Do not fire-and-forget goroutines. Make lifecycle, cancellation, and error handling explicit.
- Channels should usually be unbuffered or size one; larger buffers need a concrete reason.
- Avoid mutable globals.
- Avoid `init()` unless there is no clearer explicit wiring path.

## Names And Layout

- For variable, function, type, package, file, DTO, and helper names, apply `naming`.
- Avoid names that shadow built-ins.
- Keep import groups ordered: standard library, third-party, local project.
- Use import aliases only when they improve clarity or avoid a real conflict.
- Use recognizable standard-library constants directly. Do not rename values such as `http.StatusOK`
  behind a local constant unless the local name represents a distinct domain policy rather than the same value.
- Use blank lines to expose a function's logical phases, such as input conversion and validation, tracing,
  request construction, I/O, response conversion, evidence recording, and return. Keep a multi-line fluent
  call chain visually separated from the preceding declaration and the statement that executes or consumes it.
- Handle an error before reading or exposing the value returned alongside it.
- Group related declarations, but keep method-per-file notation when that skill applies.
- Reduce nesting with early returns.
- Do not use unnecessary `else` after a returning branch.
- For helper extraction, function splitting, or abstraction decisions, apply `function-decomposition`.

## Function Inputs And Results

Limit functions and methods to at most three parameters, excluding the receiver
but including `context.Context`. When a signature would exceed three, keep the
context separate and group the cohesive input fields into an intent-revealing
request DTO owned by the method's contract layer. A grouped declaration such
as `now, until time.Time` still represents two parameters and does not bypass
the limit. Keep functional options separate from the request DTO when they
control execution behavior rather than carry request data.

Apply the same maximum to return values. When a function would return more
than three values, group the cohesive output into an intent-revealing response
or result DTO and keep `error` separate. A grouped declaration still counts
each returned value individually. For Fx providers, use an `fx.Out` result
struct so dependency injection can continue exposing the individual outputs.

## Initialization

- Use field names when initializing structs unless the type is a tiny local value where positional initialization is obviously clearer.
- Omit zero-value fields from struct literals unless the zero value carries important meaning.
- Prefer `var value T` for zero-value structs.
- Prefer specifying capacity for slices and maps when the size is known.

## Application Configuration

The validated application configuration is an established singleton. Public
application-service, gateway, and delivery-worker constructors must read the
needed section from `config.Instance()` rather than accept `*config.Config` or
make the Fx composition root forward that singleton. Treat these layers
consistently: configuration is process state, while repositories, gateways,
transaction managers, metrics, and other behavioral collaborators remain
explicit dependencies.

This rule does not prohibit low-level technical constructors from accepting
their own narrow value configuration, such as `postgres.Config` or
`worker.Config`. It prohibits passing the global root configuration through
public runtime-component constructors.

## Performance Hygiene

- Prefer `strconv` over `fmt` for simple numeric/string conversions.
- Avoid repeated string/byte conversions in hot or repeated paths.
- Do not optimize at the cost of clarity unless there is a measured reason.
- Use maps, sets, sorting, batching, or concurrency when the data shape or task justifies them, not as default ornamentation.

## Tests

- Prefer table-driven tests for multiple cases.
- Keep test case names descriptive.
- Test behavior and boundary conditions, not implementation details.

## Review Checklist

Before finalizing Go changes, check:

- edited files are formatted
- errors are classified/wrapped consistently with project conventions
- no pointer-to-interface, avoidable globals, or unnecessary `init()`
- public runtime-component constructors read the established application config singleton instead of accepting `*config.Config`
- mutable boundary data is copied where ownership changes
- imports are clear
- names follow `naming`
- functions have no more than three parameters, excluding the receiver; larger inputs use a request DTO
- functions return no more than three values; larger outputs use a response or result DTO
- function decomposition decisions follow `function-decomposition`
- DTO tags are present only when used
- tests or compile checks cover the touched package
