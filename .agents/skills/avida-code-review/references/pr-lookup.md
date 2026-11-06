# Locating the PR

In this runtime the PR is **already identified by the trigger** — the coordinates
(`project`, `repo`, `pr_id`, `commit`) arrive in the run's input. Read the PR directly by
those coordinates for intent reconstruction (Phase 1) and post to that same PR (Phase 7).
There is **no remote to resolve, no search to run, and no branch to match** — you were
handed the exact PR.

So the usual lookup pitfalls (auto-scan, truncated negatives, guessing among siblings) do
not arise: don't search, don't disambiguate, don't ask which PR. Use what the trigger gave you.

## Only if coordinates are missing

If the input somehow lacks the coordinates (a misconfigured trigger), do not guess:

- Record in the run summary that PR coordinates were not provided and stop — there is no
  human to ask and no safe way to pick a PR blind.
- A URL of the form `.../projects/{PROJECT}/repos/{REPO}/pull-requests/{PR_ID}` in the
  input states `project`, `repo`, and `pr_id` explicitly — parse it if present.

## Reading the PR

Use the runtime's read capabilities for: PR metadata (title, description, branches,
author), the changed-files list, the diff, existing PR comments, and (when referenced)
linked tickets and related PRs. These are read-only and need no confirmation. The concrete
tools are whatever the running agent has mounted — this skill names the capability, not the tool.
