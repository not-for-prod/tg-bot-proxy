# Severity Rubric

The skill emits findings at four priorities. Each priority maps to a single action contract — never overload the rubric.

| Priority | Tag | Contract | Examples |
|---|---|---|---|
| **P0** | 🔴 release-blocker | **Block merge.** Outage, data loss, exploitable security, tenant leak, broken core flow. | SQL injection in a public endpoint; tenant predicate missing on bulk delete; migration drops a populated column. |
| **P1** | 🟠 must-fix-before-merge | **Block merge.** Wrong behaviour, broken contract, hard-to-recover bug, unrecoverable regression, missing tests for a P0-shaped scenario. | Off-by-one in retry loop on the money path; backward-incompatible response payload; race on idempotency key. |
| **P2** | 🟡 important | **Fix before next deploy.** Production-readiness gap that does not block this merge but should not ship to prod as-is. | Missing structured log on a critical branch; N+1 in a non-hot path; ungraceful failure on a rare input. |
| **P3** | 🟢 minor-but-valid | **Nice to have.** Concrete production-readiness or maintainability issue with a real, narrow impact. | New file pushes a module past the documented size limit; dead branch from a partial refactor; missing tenant id in one trace event. |

## Out of scope at any priority

These categories are **never findings** in this skill:

- **Cosmetic / style / formatting** — `prettier`, `gofmt`, `ruff format` already handle them.
- **Personal preference / "I'd write it differently"** — without a concrete reliability, security, or correctness impact.
- **Speculative future risk** — "this *might* be a problem if traffic 100×" with no evidence in the diff.
- **Unrelated cleanup / scope creep** — "while you're here, also refactor X."
- **Architectural alternatives the diff did not introduce** — the diff is judged on what it does, not on the road not taken.

## Praise is allowed (sparingly)

If the diff introduces a notably good pattern (test that nails a subtle invariant, clean refactor that paid down debt without scope creep, security-positive change), the orchestrator MAY include a short `## Praise` section in the final report. **Praise is never a finding object**, never gets a priority, and never blocks merge. One or two bullets max.

## Calibration heuristics

- A finding is P0/P1 only if you can name **the user-visible failure or exploit**, not just "this is risky".
- A finding is P2 only if it has a **named operational consequence**: "the on-call cannot debug this branch", "this query will degrade above 10k rows", "this retry will double-charge".
- P3 must still be concrete. If you cannot point at a file and line that is materially worse for production *because of this diff*, drop it.
- When unsure between two priorities, **pick the lower** and write a one-sentence justification in `body`.
- A finding cannot escalate itself: if multiple reviewers raise the same issue at different priorities, the orchestrator keeps the **highest justified** one — "justified" means the body explains the user-visible / operational impact, not just that another reviewer also raised it.

## Migration / rollout severity calibration

When the Phase 2 intent block tags the PR as `intentional migration / cleanup`, `deprecation rollout`, or `split rollout`, a finding can be P0/P1 **only** when it shows one of:

- **Intent violation** — the diff does something the stated intent did not authorise.
- **Parity loss** — observable behaviour changed in a way the intent did not say to change.
- **Safety-net gap** — between rollout steps, callers can hit a broken path with no fallback / flag / dual-write.
- **Missing sibling rollout** — the corresponding consumer-side (or producer-side) PR is not referenced or in flight.

Without one of those, demote the finding or reframe it as a non-bug category (see below). Do **not** raise P0/P1 simply because legacy code was removed.

## Non-bug categories

Some findings describe coordination risks, not bugs. The orchestrator may surface them in the report under priority sections, but they are framed differently in the `body`:

- **`rollout dependency`** — "this PR is safe only if PR/issue X ships first/together".
- **`migration note`** — heads-up about a deliberate behaviour change that downstream consumers should know about.
- **`parity risk`** — flag where new behaviour might differ from old in a way the author should explicitly confirm.
- **`config completeness`** — required platform-config file (auth.toml / nfr.toml / app.toml / alert.yaml) not synchronised with new entrypoint (see `references/avito/config-completeness.md`).

These are still subject to the priority rubric, but their `body` must lead with the category framing, not with "bug:".

## Tag-priority cross-walk for inline PR comments

When the skill posts to a PR (Stash / Bitbucket / GitHub), it uses these tags so authors can prioritise visually:

- 🔴 P0 — opens a blocking task.
- 🟠 P1 — opens a blocking task.
- 🟡 P2 — opens a non-blocking comment.
- 🟢 P3 — opens a non-blocking comment.

Praise, when posted, is a single PR-level comment, not inline.

## Maintainability → severity cross-walk

The maintainability standard (ambition directive, output-ordering ladder, approval bar, presumptive
blockers) is defined once in `references/maintainability.md`. This is the **only** place that maps it
onto P0–P3 — do not restate the ladder or approval bar here:

- A presumptive-blocker condition (per `maintainability.md`) that the author has **not** justified via the Phase 2 intent block → minimum **P1**.
- A presumptive-blocker that also crosses correctness / security / data-integrity → **P0**, governed by the standard P0 contract above.
- A condition cleared by author justification (read from PR description / linked ticket) → demote per this rubric.
- This is the single rule for when `simplicity` / `maintainability` exceed their usual ceiling — there is no other trigger. Absent an unjustified presumptive blocker, they stay ≤ P1.
- The output-ordering ladder in `maintainability.md` defines **report ordering** inside each priority section (structural first, nits last); it does not change priority.
