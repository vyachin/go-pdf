# Proposal

## Why

Every push to a feature branch starts two identical CI runs: one from the `push`
event and one from the `pull_request` event of the pull request that covers the
same commit. Pull request #9 shows the pattern: each push produced a pair of
`lint`/`test` runs for the same tree, doubling the minutes and leaving two
statuses of record for one change.

The pipeline should cover what is about to land and what has landed: a commit on
`main`, and a pull request.

## What Changes

- `push` triggers are limited to the `main` branch; a push to any other branch
  no longer starts a run of its own, because its pull request covers it.
- `pull_request` keeps firing for every pull request, whatever its base branch.
- The workflow stays a single file with one job graph; concurrency, pins and
  steps are unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the two requirements that say CI runs "on every push and pull
  request" now say "on every push to `main` and on every pull request", and a new
  requirement pins one run per change.

## Impact

- `.github/workflows/ci.yml` — the `on:` block only.
- No code, no dependencies, no job steps. The required status checks on `main`
  keep working because the `pull_request` run is unchanged.
