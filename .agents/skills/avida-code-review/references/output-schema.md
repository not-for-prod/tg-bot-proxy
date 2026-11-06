# Findings Output Schema

Every reviewer agent (and the orchestrator merge step) must emit findings in this exact JSON shape — **no markdown fences, no preamble, no trailing text**.

## Top-level object

```json
{
  "findings": [ /* see Finding shape below */ ],
  "overall_correctness": "patch is correct" | "patch is incorrect",
  "overall_explanation": "1–3 sentences summarising the patch",
  "overall_confidence_score": 0.0–1.0,
  "self_check": {
    "guard_passed": true,
    "skipped_speculative_count": 0
  }
}
```

If nothing production-relevant is found, return `"findings": []` and `"overall_correctness": "patch is correct"`. **Never invent issues to fill the array.**

`overall_correctness` is a best-effort signal. Roles that do not judge overall correctness (e.g. `code-simplifier`, `frontend-developer`, `sql-pro`) should base it only on what their lens can see and default to `"patch is correct"` when correctness is outside their remit — the value is advisory, not a gate.

`self_check.guard_passed` must be `true` — it is the reviewer's affirmation that every emitted finding cleared the five-point check in `references/false-positive-guard.md`. `self_check.skipped_speculative_count` is the number of candidate issues the reviewer considered but dropped under that guard (useful for calibration; honest numbers preferred to zero).

## Finding shape

```json
{
  "title": "[P1] Preserve tenant isolation when copying experiment settings",
  "body": "One-paragraph plain description of the issue and what makes it wrong.",
  "priority": "P0" | "P1" | "P2" | "P3",
  "category": "<one of categories from references/categories.md>",
  "absolute_file_path": "/absolute/path/to/file",
  "line_range": { "start": 42, "end": 42 },
  "confidence_score": 0.0–1.0,
  "introduced_by_diff": true,
  "production_impact": "Concrete user-visible / operational impact. Not 'this is risky'.",
  "evidence": {
    "snippet": "≤5 lines quoted from the actual file or null",
    "convention_source": "AGENTS.md / CLAUDE.md / sibling file / doc URL / null",
    "reachability_note": "Why the path is reachable — guards acknowledged",
    "enclosing_diff_line": "a changed line in the same enclosing function for a latent finding, else null"
  },
  "suggested_fix": "One concrete change, optionally with a small code suggestion. Null if not obvious.",
  "agents": ["architect-reviewer"]
}
```

### Field rules

- `priority`: must be `P0`, `P1`, `P2`, or `P3` (see scale below and `references/rubric.md`).
- `category`: must be one value from `references/categories.md`.
- `line_range`: `start` ≤ `end`, both in the **post-image** (added/context) line numbering of the diff. The range **must overlap** at least one changed line in the diff — **unless** this is a *latent finding* in a touched function (see `evidence.enclosing_diff_line` below), where `line_range` may point at an unchanged line.
- `confidence_score`: 0.5 = weak, 0.8 = strong, 0.95+ = essentially certain. The orchestrator may reject findings below 0.6 if they did not survive verification — a **post-verification, discretionary** filter, distinct from the pre-verification orchestrator-side rejections listed below.
- `introduced_by_diff`: `true` only if the diff added or materially exposed this issue. Pre-existing untouched issues → `false` and usually skip reporting.
- `production_impact`: a sentence the on-call could quote in a postmortem. "Bad practice" / "could be confusing" do not pass.
- `evidence.snippet`: ≤5 lines, quoted verbatim from the file. If the issue is structural (file too large, missing file), explain in `convention_source` instead.
- `evidence.convention_source`: when the finding cites a project rule, name the file/section. If no such rule exists, the convention claim must be dropped.
- `evidence.reachability_note`: for correctness/security/race/perf, name the entrypoint and any guards that exist on the path (so the verifier can confirm reachability fast). Set `null` for structural / maintainability / simplicity findings where reachability does not apply — do not invent a path.
- `evidence.enclosing_diff_line`: normally `null`. Set it **only** for a *latent finding* — a real bug on an **unchanged** line inside a function this diff touches, lying too far from any changed line for `line_range` to overlap. Then `line_range` points at the latent bug, `introduced_by_diff` is `false`, and `enclosing_diff_line` names a **changed** line in the **same enclosing function** (the diff anchor proving the function is in scope); `evidence.snippet` quotes the latent line. Allowed **only** for categories `correctness`, `security`, `tenant_isolation`, `data_integrity`, `reliability`, `business_logic`, `maintainability`. For `simplicity` / `dead_code` / style, `line_range` must still overlap a changed line. For `maintainability` this is consistent with the Hybrid scope rule in `references/maintainability.md`: `enclosing_diff_line` is the required diff anchor, the structural smell is cited at `line_range`.
- `suggested_fix`: actionable — what to change, not "improve this". Optional.
- `agents`: list of reviewer-agent names that surfaced this finding. Populated by the orchestrator after dedupe.
- `title`: starts with `[Px]`, then a short imperative description.

## Priority scale

- **P0** — release-blocking outage, security breach, data loss / corruption, tenant or auth leak.
- **P1** — must fix before merge: broken contracts, hard-to-recover bug, regression in core flow. (Overengineering / maintainability reach P1 only via the maintainability cross-walk in `references/rubric.md` — an unjustified presumptive blocker.)
- **P2** — important production-readiness degradation that should be fixed before next deploy.
- **P3** — minor but valid issue with concrete production-readiness impact (not style/taste).

Cosmetic style, personal preference, broad unrelated cleanup, optional refactors, and speculative risks without code evidence are **out of scope** — do not report them at any priority. See `references/rubric.md` for the full out-of-scope list.

## After fact-check

The orchestrator augments each finding with:

```json
{
  "verdict": "confirmed" | "unconfirmed" | "rejected",
  "verifier_evidence": ["paths/snippets/docs consulted"],
  "verifier_reasoning": "1–3 sentence justification",
  "verifier_confidence": 0.0–1.0
}
```

Only `confirmed` findings are reported to the user. `unconfirmed` findings go to a clarification round (one batched user question). `rejected` findings are discarded — never surfaced in the report.

## Orchestrator-side rejection (before fact-check)

The orchestrator rejects a finding before sending it to the verifier if:

- `priority` not in `P0..P3` **or** `category` not in `references/categories.md`.
- `line_range` does not overlap any changed line in the actual diff — **except** a valid latent finding: `evidence.enclosing_diff_line` is set, itself overlaps a changed line, and `category` is in the latent-allowed set (`correctness`, `security`, `tenant_isolation`, `data_integrity`, `reliability`, `business_logic`, `maintainability`). A non-overlapping `line_range` without a valid `enclosing_diff_line`, or with a category outside that set, is rejected.
- `evidence.snippet` is null **and** `evidence.convention_source` is null (no anchor at all).
- `production_impact` is one of: "could be a problem", "might fail", "bad practice", "not ideal", "consider…" — these strings indicate no concrete impact.

Rejected-by-orchestrator findings are not retried and are discarded silently — not surfaced in the report.
