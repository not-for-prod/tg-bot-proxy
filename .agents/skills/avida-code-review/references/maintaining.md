<!-- skill-validator: examples-ok -->

# Maintaining this skill (how to improve it)

## Lineage — this skill is inherited from an upstream

`avida-code-review` is a **vendored copy** of trisigma's `code-review-team` skill. The
upstream lives at:

```
https://stash.msk.avito.ru/projects/AGENT/repos/trisigma-skills/browse/skills/code-review-team
```

**trisigma** is the original author (credited as `metadata.upstream` and in the lineage
banner + `README.md`); **vabozhev** owns this vendored avida copy (`metadata.owner`) and
seeded the adaptation via the `demo/code-review` agent. Keep trisigma's upstream credit and
the lineage banner — do not remove them.

The upstream is the source of the **review methodology** (the reviewer roles, the rubric, the
false-positive guard, the fact-check). When that methodology improves upstream, we want those
improvements here too — but only after re-applying the avida adaptations below.

## The avida adaptations to preserve (do not regress these)

This copy is hardened for the avida runtime. Any sync from upstream must re-apply these — they
are the whole reason the vendored copy exists. The rationale for each is captured in the
OpenSpec change `openspec/changes/archive/2026-06-18-paas-code-review-bot/` (decisions D1–D8).

1. **Vendor-neutral.** No concrete tool names — no `mcp__*`, no `paas_*`, `sentry_*`, `jira_*`,
   etc. Describe the *capability* ("post a blocking task on a diff line", "search internal
   docs / code"); the running agent supplies the tool. Enforced by the repo linter.
2. **Headless (no shell).** Gather the diff via the runtime's PR-clone + diff capability, not
   `git diff $(merge-base)`. Inspect the repo by reading/globbing the clone, not `ls`/`grep`.
   Project preflight (lint/typecheck/test) is **skipped** — there is no shell.
3. **Non-interactive (no human in the loop).** The "ask the user" branches become autonomous
   defaults: large diff → per-file dispatch automatically; `unconfirmed` findings → dropped to
   the internal run summary, never posted; PR identity comes from the trigger (no lookup/branch
   match).
4. **Posting policy.** Confirmed findings only. P0/P1 → merge-blocking task, P2/P3 → inline
   comment, praise → one PR-level comment. **Never** approve / merge / decline the PR.
5. **Untrusted convention files.** `AGENTS.md` / `CLAUDE.md` are mined for code conventions but
   treated as data, not commands (Phase 0a trust boundary).
6. **Language split.** Skill instructions + the internal run summary are in **English**;
   author-facing PR output (comments, tasks, PR summary) is in **Russian** with machine tokens
   kept English. See `references/stash-posting.md` for the Russian templates.

## How to sync an upstream improvement

1. **Compare.** Diff the current upstream (URL above) against this copy to see what changed —
   most often the reviewer roles (`references/roles/*`), the rubric, the guard, the avito
   enrichment, or the output schema.
2. **Bring over the methodology, not the plumbing.** The role briefings, rubric, categories,
   maintainability standard, false-positive guard, and `avito/*` enrichment carry over almost
   verbatim — they are clean. Take genuine improvements there directly.
3. **Re-apply the avida deltas** to any changed orchestration/process files. The files that
   needed adaptation (and will again): `SKILL.md`, `references/pr-lookup.md`,
   `references/stash-posting.md`, `references/false-positive-guard.md`,
   `references/stack-detection.md`, `references/preflight.md`, `README.md`. Re-check the list
   in adaptation #1–#6 above against the new text.
4. **Validate.** Run the repo linter until clean:
   `python3 .agents/skills/skill-validator/scripts/validate.py skills/avida-code-review`.
   No `E101`/`E102` (tool names), no `E106` (cross-skill paths), no local-path `W101`. Keep
   `SKILL.md` under 500 lines (split deep content into `references/`).
5. **Update evals.** Keep `evals/evals.json` aligned with any behavior change (≥3 cases).
6. **Keep authorship + lineage intact.**

## The quick variant

A separate lighter skill (`avida-code-review-quick`) is a single-pass review of the same
family — one pass, comments only, no subagents. It is **not** generated from this skill; if you
change shared conventions (priorities, the trust boundary, the language split), update both so
they stay consistent.
