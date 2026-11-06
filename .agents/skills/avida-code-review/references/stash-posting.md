# Phase 7 — Post Findings to the PR

This phase posts the **confirmed** findings back to the PR that triggered the run. It is
not gated on a human's confirmation — the trigger *is* the request to post. The PR
coordinates are the handed-in ones (see `pr-lookup.md`); there is no PR to choose and no
branch to match.

This skill names the **capabilities** below; the concrete tools are whatever the running
agent has mounted for reading and writing a PR. If no PR-write capability is available,
skip posting and record that in the run summary.

## What gets posted

Post **only** `verdict=confirmed` findings (Phase 5). Never post `unconfirmed`, dropped, or
rejected findings, and never any review-process meta.

| Priority | How to post | Effect |
|---|---|---|
| P0 / P1 | **merge-blocking task** on the diff line | holds the merge until a human resolves it |
| P2 / P3 | **non-blocking inline comment** on the diff line | advisory |
| Praise (≤2) | single PR-level summary comment | never inline |

The bot **never approves, merges, or declines** the PR. A blocking task is the only gating
mechanism, and resolving it stays with humans.

## Step 1 — Fetch the annotated diff

Read the PR's diff (the same PR coordinates). For large diffs, fetch per file. The diff
carries line markers you need to anchor comments:

- `+[new:14] ...` → added line → `line=14, line_type=ADDED, file_type=TO`
- `-[old:7] ...` → removed line → `line=7, line_type=REMOVED, file_type=FROM`
- `[old:3,new:5] ...` → context line → `line=5, line_type=CONTEXT, file_type=TO`

## Step 2 — Map each confirmed finding to a diff line

1. Locate the finding's file in the PR diff.
2. Pick the diff line closest to the reported line number.
3. Determine `line_type` and `file_type` from the marker.
4. **Skip unmappable findings** (file not in the PR diff, or line not visible) and remember
   them for the summary section — never post them as general comments.

## Step 3 — Post the comments

For each mapped finding, post on its diff line: P0/P1 as a **merge-blocking task**, P2/P3 as
a **non-blocking inline comment**. Provide the file path (from the diff's post-image `+++`
path), the line, and (for comments) the `line_type` / `file_type` resolved above.

### Comment / task text template

The PR comment text is written in **Russian** (PR output is author-facing Russian — see
SKILL.md Phase 7); keep `{PRIORITY}` / `{CATEGORY}` tokens and the label words below in the
literal form shown. Fill `{...}` placeholders with Russian prose.

```markdown
**[{PRIORITY}] [{CATEGORY}]** {описание одним абзацем}

Влияние на прод: {production_impact}

{предлагаемый фикс, если есть}
```

The comment carries only the problem, its impact, and (if any) the fix — nothing else. No
review process in the text: do not attach verification/fact-check status, do not include
confidence scores, do not reference the verifier. Only `confirmed` findings are posted, so a
fact-check note is redundant.

## Step 4 — Post the summary comment

After the inline comments/tasks, add one PR-level comment (no file/line):

```markdown
## Итоги ревью кода

**Ветка**: {branch_name}
**Ревьюеры**: {comma-separated list of reviewer roles actually used}

| Приоритет | Кол-во |
|-----------|--------|
| P0 | {n} |
| P1 | {n} |
| P2 | {n} |
| P3 | {n} |
| Пропущено (вне диффа) | {n} |

{2–3 предложения: готов ли PR к мёрджу и что починить в первую очередь — только по
подтверждённым находкам.}
```

The summary is actionable only. No review process: do not mention fact-check/verification,
do not list rejected/dropped findings, do not give reasons. That is useless to the author —
they want to know what to fix.

## Error handling

- **PR-write capability unavailable** → skip posting, record it in the run summary, do not retry.
- **Auth error** → record it in the run summary; do not retry silently.
- **Line not in diff** → skip the finding, count it under "Пропущено" in the summary.
- **Many comments** → pace the calls to avoid rate-limiting.
- **Re-run dedup** → optionally read existing PR comments first and skip ones already present from a prior run.

## Rules

- Inline comments/tasks target **specific diff lines**, not the whole PR.
- `file_path` always comes from the diff's post-image (`+++`) path.
- Renamed files: REMOVED-line comments may fail (API limitation) — skip them.
- Unmappable findings → never post as general comments; list them only in the summary.
- No confirmed findings → post only the summary confirming a clean review; create no tasks.
- **PR output is author-facing and actionable only.** Never surface the internal review
  process anywhere in a comment, task, or summary: no fact-check / verification status, no
  verifier reasoning, no confidence scores, no dropped or rejected findings, no "why X
  wasn't reported". That meta lives only in the Phase 6 run summary.
- **Never comment about the bot's own internal failure in the PR** (no PR access, empty
  diff, tool unavailable) — record it in the run summary instead.
