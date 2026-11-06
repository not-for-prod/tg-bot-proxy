# Maintainability Standard (shared)

The canonical maintainability / code-quality doctrine for every reviewer in this skill. Embedded
into the shared review context (Phase 2). Read once, treat as authoritative. **Role-specific
applications of rules 1–7 live in each agent's own prompt** — this file holds only the common part.

## Core directive

Above all, be **ambitious** about code structure. Do not merely identify local cleanup. Actively
search for structural moves: restructurings that preserve behavior while making the implementation
dramatically simpler, smaller, more direct, and more elegant.

Baseline every reviewer adopts:

> Perform a deep code quality audit of the current branch's changes.
> Rethink how to structure / implement the changes to meaningfully improve code quality without impacting behavior.
> Work to improve abstractions, modularity, reduce Spaghetti code, improve succinctness and legibility.
> Be ambitious, if there is a clear path to improving the implementation that involves restructuring some of the codebase, go for it.
> Be extremely thorough and rigorous. Measure twice, cut once.

### Scope (Hybrid — how ambition meets the diff)

A structural finding **must anchor to a line changed by this diff** (the trigger that introduced or
materially exposed the smell). The ambitious restructuring — which may legitimately touch
surrounding code — goes in `suggested_fix`, not in `line_range`. A structural improvement whose only
target is unchanged code with no diff anchor is out of scope for an inline finding; the orchestrator
may still surface it in the PR-level summary. This is what makes "go for it" compatible with a
diff-scoped, postable, fact-checkable pipeline.

## Rule 0 — Be ambitious about structural simplification

- Do not stop at "this could be a bit cleaner."
- Look for opportunities to reframe the change so that whole branches, helpers, modes, conditionals, or layers disappear entirely.
- Prefer the solution that makes the code feel inevitable in hindsight.
- Assume there is often a structural move available: a re-organization that uses the existing architecture more effectively and makes the change dramatically simpler and more elegant.
- If you see a path to delete complexity rather than rearrange it, push hard for that path.

Rules 1–7 are applied per role; each reviewer's prompt carries the rules relevant to its specialty:

1. **File-size ceiling** — don't let a PR push a file from under 1k lines to over 1k without a very strong reason.
2. **No spaghetti growth** — no ad-hoc conditionals / scattered special cases bolted into existing flows.
3. **Clean the design** — bias toward the cleaner structure, not just accepting working code.
4. **Boring over magical** — prefer direct, boring, maintainable code; flag thin wrappers and magic.
5. **Type & boundary cleanliness** — question unnecessary optionality / `any` / casts; make boundaries explicit.
6. **Canonical layer & reuse** — keep logic in the canonical layer; reuse existing helpers.
7. **Orchestration & atomicity** — unnecessary sequential orchestration and non-atomic updates are design smells.

## Primary review questions

For every meaningful change, ask:

- Is there a structural move that would make this dramatically simpler?
- Can this be reframed so fewer concepts, branches, or helper layers are needed?
- Does this improve or worsen the local architecture?
- Did the diff add branching complexity where a better abstraction should exist?
- Did a previously cohesive module become more coupled, more stateful, or harder to scan?
- Is this logic living in the right file and layer?
- Did this change enlarge a file or component past a healthy size boundary?
- Are there repeated conditionals that signal a missing model or missing helper?
- Is the implementation direct and legible, or does it rely on special cases and incidental control flow?
- Is this abstraction actually earning its keep, or is it just a wrapper?
- Did the diff introduce casts, optionality, or ad-hoc object shapes that obscure the real invariant?
- Is this logic living in the canonical layer, or did the diff leak details across a boundary?
- Is this orchestration more sequential or less atomic than it needs to be?

## Output expectations (report ordering)

Prioritize findings in this order:

1. Structural code-quality regressions
2. Missed opportunities for dramatic simplification / structural restructuring
3. Spaghetti / branching complexity increases
4. Boundary / abstraction / type-contract problems that make the code harder to reason about
5. File-size and decomposition concerns
6. Modularity and abstraction issues
7. Legibility and maintainability concerns

Do not flood the review with low-value nits if there are larger structural issues. Prefer a smaller
number of high-conviction comments over a long list of cosmetic notes.

## Review tone

Be direct, serious, and demanding about quality. Do not be rude, but do not soften major
maintainability issues into mild suggestions. If the code makes the codebase messier, say so
clearly. If the implementation missed an opportunity for a dramatic simplification, say so too.

## Approval bar

Do not approve merely because behavior seems correct. The bar for approval:

- no clear structural regression
- no obvious missed opportunity to make the implementation dramatically simpler when such a path is visible
- no unjustified file-size explosion
- no obvious spaghetti-growth from special-case branching
- no obviously hacky or magical abstraction that makes the code harder to reason about
- no unnecessary wrapper/cast/optionality churn obscuring the real design
- no clear architecture-boundary leak or avoidable canonical-helper duplication
- no missed opportunity for an obvious decomposition that would materially improve maintainability

Treat these as presumptive blockers unless the author justifies them clearly:

- the PR preserves a lot of incidental complexity when a plausible structural move would delete it
- the PR pushes a file from below 1000 lines to above 1000 lines
- the PR adds ad-hoc branching that makes an existing flow more tangled
- the PR solves a local problem by scattering feature checks across shared code
- the PR adds an unnecessary abstraction, wrapper, or cast-heavy contract that makes the design more indirect
- the PR duplicates an existing helper or puts logic in the wrong layer when there is a clear canonical home

### Mapping into this skill's severity

- A presumptive-blocker condition the author has **not** justified → minimum **P1**.
- A presumptive-blocker that also crosses correctness / security / data-integrity → **P0** (standard P0 contract).
- A condition cleared by author justification → demote per `rubric.md`.
- When a presumptive blocker is not justified, leave explicit, actionable feedback and push for a cleaner decomposition — name the structural target, not just "refactor".

## How this maps to the report (not an interactive gate)

This standard feeds the **final report**, not a separate approval dialogue. Reviewers raise findings;
the orchestrator escalates unjustified presumptive blockers per the mapping above and reflects the
approval bar in the report's Summary (the merge recommendation). "Author justification" is whatever
the Phase 2 intent block read from primary sources (PR description / linked ticket) — there is no
separate back-and-forth with the author during fact-check.
