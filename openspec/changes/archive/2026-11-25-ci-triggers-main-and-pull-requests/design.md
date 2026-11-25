# Design

## Context

The workflow currently declares `on: push:` and `on: pull_request:` without
filters. GitHub starts both events for one commit that is pushed to a branch with
an open pull request, so the same tree is verified twice and two checks of record
appear for one change. See proposal.md.

The job graph, the pins, the concurrency group and the required status checks are
out of scope: only the `on:` block changes, and job names stay identical so any
branch protection that references them keeps working.

## Goals / Non-Goals

**Goals:**

- One run per change: the pull request run while a change is under review, the
  `push` run once it lands on `main`.
- No event other than those two starts the workflow.

**Non-Goals:**

- Path filters, so a documentation-only change still runs the pipeline.
- A release or nightly workflow; tags and schedules are deliberately not part of
  this configuration.
- Changes to the concurrency group or to the required status checks.

## Decisions

### D1. `push` is restricted to `main`; `pull_request` stays unfiltered

A feature-branch push is already covered by the pull request that contains it.
Filtering `pull_request` by base branch would drop verification for a pull
request against any other branch, and the requirement says every pull request is
verified, so the filter goes on `push` only.

### D2. No `paths` filters

A documentation change can break a test that asserts a documented default, so the
pipeline runs for it. Filters would also make the required checks depend on the
diff, which is exactly the kind of surprise a required check must not have.

### D3. No `workflow_dispatch`

A run that needs repeating can be re-run from the pull request or the commit,
which reuses the same configuration and shows up in the same place.

### D4. Tags do not start a run

A release tag points at a commit that was already verified on `main`; re-running
the pipeline for the tag would duplicate that verification without adding
information.

## Risks / Trade-offs

- [A direct push to `main` still starts a run, and that is intended] → the run is
  the only verification such a commit gets.
- [Branch protection may name the `push` run of a feature branch] → protection
  already requires the pull request run; job names are unchanged, so the checks
  keep matching.
- [A pull request from a fork] → `pull_request` still triggers, and the workflow
  requests no secrets beyond the default read-only token.

## Migration Plan

One commit changes the `on:` block and records this change. Verification: push to
the open pull request and confirm one new run, not two; after the merge, the
merge commit on `main` starts one run. Rollback is the reverse edit.

## Open Questions

None.
