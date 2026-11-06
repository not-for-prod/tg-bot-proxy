---
name: function-decomposition
description: Prevent over-decomposition and decide when a function may be split, moved, or abstracted. Use whenever Codex considers extracting a helper or callback, reviews a function whose cognitive complexity is greater than 20 or whose length is greater than 60 lines, or proposes an extraction to remove proven code duplication.
---

# Function Decomposition

Keep readable behavior in one place. Unnecessary helpers force readers to jump between functions and hide the business and domain-event flow.

## Hard Gate

Do not decompose a function or method unless at least one condition is true:

- cognitive complexity is greater than 20
- the function body is longer than 60 lines
- the extraction removes proven existing code duplication

Treat both thresholds as strict `>` comparisons. Cognitive complexity 20 and exactly 60 lines stay inline.

Passing the gate permits decomposition; it does not require it. Keep even a longer straight-line orchestration function inline when splitting it would make the complete flow harder to follow.

Existing interface methods, framework callbacks, and functions required by generated contracts are not optional helper extraction and are not governed by this gate.

## Default

Keep code inline. Below the hard gate, do not extract:

- shared read-modify-write transaction helpers
- one-use helpers
- helpers that only shorten the caller
- helpers for a small repeated block without proven duplication
- callback-based transition wrappers
- speculative reusable abstractions
- helpers added merely to name a few obvious lines

Prefer a small amount of explicit duplication over hiding a short business flow.

Do not preserve an unjustified helper merely because its argument list violates
the function-argument limit. The argument-count rule is not evidence that the
helper represents a useful abstraction. Apply this decomposition gate first:
when a small helper fails it, inline the behavior and remove the helper instead
of introducing a request/config DTO whose only purpose is to keep the helper's
signature under the limit.

For an aggregate command, keep this sequence visible in the public method:

1. load and lock the aggregate
2. call the aggregate method and receive `domainEvent`
3. return on error or event-free no-op
4. persist the aggregate
5. publish that exact domain event

Do not replace that visible sequence with a generic `update`, `mutate`, `transition`, or `do` helper when the method is below the gate.

## Duplication Exception

Allow decomposition below both thresholds only when all of these facts are shown before editing:

- at least two existing production call sites contain the same meaningful behavior
- the duplicated statements and branches are identified by file and function
- the proposed extraction removes duplicated production code in the resulting diff
- the helper has one precise responsibility and an intent-revealing name
- the extraction does not hide a business workflow, aggregate transition, persistence step, or domain-event publication

Repeated one-line expressions do not qualify as meaningful duplicated behavior
when replacing each expression with a one-line helper call leaves caller
complexity unchanged and the helper declaration increases the total code size.
Count the helper signature and braces when comparing the before/after line
usage. Keep short error construction and classification expressions inline so
the returned error kind remains visible at its origin; do not extract a
helper merely to abbreviate repeated `errors.Join(...)` calls, whether the
helper is parameterless or accepts values used only inside that expression.

One-line error predicate wrappers are an explicit anti-example:

```go
// Bad: adds navigation without reducing the call site or expressing policy.
func isNotFound(err error) bool {
    return errors.Is(err, &proterror.NotFound{})
}

// Good: keep the classification visible where it controls the branch.
if errors.Is(err, &proterror.NotFound{}) {
    // ...
}
```

Apply the same rule to helpers that only forward `errors.As`, compare one
sentinel, or negate one classification check. A predicate helper is justified
only when it combines meaningful policy that is clearer and smaller behind a
stable name—not when it merely renames a standard-library expression.

When review finds one trivial construction helper, audit production code for
other helpers that only return a single error-construction, wrapping,
classification, or predicate expression. Apply the same reverse gate to every
match instead of removing only the cited helper.

Use repository search and the before/after diff as proof. Similar-looking code, possible future reuse, a one-use helper, duplicated tests, or fewer caller lines alone are not proof.

State the proof in the work update or final response: name the call sites and what duplicated behavior the extraction removed. If that evidence cannot be stated concretely, keep the code inline.

Apply the same gate in reverse during review: remove single-use pass-through helpers that only rename one call. Shared technical primitives may use the duplication exception when repository search proves at least two production copies—for example, identical event-context mapping or bounded response reading—but the extracted primitive must not conceal an aggregate transition or use-case step.

## Review After The Gate

When a function passes the gate, first classify the source of complexity:

- domain policy belongs on the aggregate or value object
- provider, SQL, transport, signing, or conversion mechanics belong at their owning boundary
- multiple real phases may become focused functions
- straight-line orchestration may remain inline

Extract only when the result is easier to understand end to end. Every helper must name a real concept or ownership boundary. Reject vague helpers such as `update`, `process`, `prepareData`, `handleResult`, or `do`.

## Measuring

- Count the function body from its opening brace through its closing brace; exclude comments and blank lines.
- Use the repository linter's cognitive-complexity result when available.
- If no tool reports cognitive complexity, conservatively keep the function inline unless its structure clearly exceeds 20.
- Do not estimate a high score merely to justify a preferred extraction.

## Final Check

- Did every new extraction pass cognitive complexity `> 20`, length `> 60`, or the complete proven-duplication exception?
- If using the duplication exception, are at least two original production call sites and the removed duplicated behavior documented?
- Can the public use case be understood without navigating to a trivial helper?
- Is the aggregate-returned domain event visible before persistence and publication?
- Is any remaining decomposition based on a real phase or ownership boundary?
- Does each duplication-based helper reduce total code or isolate genuinely non-trivial behavior, rather than replace one line with one line?
