# Design

## Context

See `proposal.md` - Why. The constraints that shape the approach are all in the current tree:

- `ci.yml` pins four action references, and the two jobs disagree on majors (`test` uses `checkout@v7` and `setup-go@v7`; `lint` uses `@v4` and `@v5`).
- The runner image `ubuntu-24.04` carries the poppler that produced the golden rasters, so a bump is a deliberate commit with regeneration (`docs/golden.md`).
- `pdfcpu@v0.11.0` is pinned because v0.12 needs Go 1.25 while the module declares `go 1.24`; the two move together.
- The `golangci-lint` version is a documented pin under the `code-quality` spec, and the README names it.
- `go.mod` declares no requirements and there is no `go.sum`, so the module ecosystem has nothing to report today.
- The default branch is `main`, and Dependabot reads its configuration only from the default branch.

## Goals / Non-Goals

**Goals:**

- One reviewable mechanism for action references, with routine bumps batched and majors isolated.
- A written boundary between what automation may touch and what a person must decide.
- A configuration that stays correct when the module gains its first requirement.

**Non-Goals:**

- Automating the coupled pins (image, poppler, `pdfcpu`, linter version, `go` directive).
- Auto-merge, auto-approve, or any workflow that acts on its own pull requests.
- Branch protection changes: `test` and `lint` are already expected to be required checks.
- Adopting Renovate or adding a self-hosted bot.
- Changing what the CI jobs do; only the action references move.

## Decisions

### 1. Dependabot, scoped to action references

Dependabot is native to the hosting platform, needs no third-party application and no workflow of its own, and its `github-actions` ecosystem covers every `uses:` reference in `.github/workflows/` without an entry per action. Renovate would additionally reach the `pdfcpu` and linter pins through regex managers - precisely the updates that must stay manual - so its extra reach is a liability here, not a feature. The alternative of no automation plus a checklist was rejected because the drift between the two jobs is already proof that a checklist is not read.

### 2. Weekly, with pruning on release age

Actions release often enough that monthly would leave a major unseen for a quarter, while daily would spend CI cycles on patch releases. A weekly check with `cooldown` (`default-days: 7`, `semver-major-days: 30`) keeps a freshly published release from becoming the first environment that runs it; a regression then has days to surface upstream before it reaches this workflow. The cooldown values are a starting point, not a commitment - see Open Questions.

### 3. Two groups: routine and major

```yaml
groups:
  actions-minor: { patterns: ["*"], update-types: ["minor", "patch"] }
  actions-major: { patterns: ["*"], update-types: ["major"] }
```

A single group for everything would let a major ride along with routine bumps, and the reviewer would approve the batch for the patch inside it. No groups at all would open one pull request per reference - four or more at once, each competing for the same check run. Two groups give one cheap routine pull request and one deliberate major pull request.

### 4. No auto-merge, at all

Auto-merging dependency pull requests would make the required checks the only gate on a change that can alter how the checks themselves run: a bumped `checkout` or `upload-artifact` changes the CI machinery, not just a library. The repository therefore does not add `dependabot/fetch-metadata` or any approval step, and a dependency pull request merges only through the same review path as any other. Enabling auto-merge in the repository UI is outside file control; the spec states the intent and the workflows are checked for absence of merge automation.

### 5. The module entry is active, not commented out

`go.mod` has no requirements, so the `gomod` entry reports nothing and costs one no-op check per week. It is kept active rather than commented out because a commented entry rots silently, and because the first requirement added to the module should be watched by default rather than by remembering to uncomment something. The trade-off is that this entry has no observable output until the first dependency exists - an accepted blind spot, since the first requirement that lands is reviewed as an ordinary change and its bump arrives as one more pull request.

### 6. Manual pins are named individually

The boundary is expressed as a list of concrete pins rather than a general principle, because the general principle ("automation must not break the build") is what every accidental automation violates. The list lives next to the existing toolchain requirement in `code-quality`, and the README paragraph repeats it for the contributor who never opens the spec.

### 7. Major alignment arrives with the first major pull request

The drift is a pair of major bumps (`lint`'s `checkout@v4` and `setup-go@v5` against `test`'s `@v7`), so the change does not have to special-case it: it lands in the major group, whose one pull request moves both references to the majors `test` already uses. That keeps the spec's "one major per action" requirement satisfiable through the normal review path. The manual-alignment task covers the case where that pull request is rejected or delayed, and it was taken here: the references were aligned by hand, because the configuration cannot run before it reaches `main`, so the requirement never waits on Dependabot's behaviour.

### 8. Commit message and labels

`commit-message.prefix: ci` keeps the history readable next to `docs(openspec)` and `feat:` commits, and the `dependencies` label makes the pull requests selectable as a group. Neither affects behaviour; both were chosen to match the repository's existing commit style.

## Risks / Trade-offs

- **A major bump changes action behaviour and fails CI** → major updates are isolated in their own pull request and delayed by a 30-day cooldown; the failing required check blocks the merge and the reviewer reads the release notes rather than the diff alone.
- **A grouped routine pull request cannot be merged partially** → one broken bump blocks the other routine bumps; the failing check names the action, and splitting the group per action is a one-line change if it happens repeatedly.
- **The configuration only takes effect from `main`** → merging to a feature branch proves nothing; acceptance is stated as a check after merge to the default branch.
- **The runner image or poppler changes without regeneration** → no automation touches them, and the golden render test fails with both renderer versions named if someone bumps them by hand without regenerating.
- **`pdfcpu` drifts from the Go minimum** → it stays manual precisely because the correct bump is a two-file decision (`go.mod` plus the install line), which no bot can make.
- **Auto-merge enabled in the UI later** → not detectable from the tree; the README paragraph and the spec make the intent explicit for whoever reviews repository settings.
- **The dormant `gomod` entry hides a future misconfiguration** → it stays dormant only while `go.mod` is empty; the first requirement task re-checks that the watcher picked it up.

## Migration Plan

1. Add the configuration on a branch and validate it as YAML; the change is inert until it reaches the default branch.
2. Merge to `main`.
3. Expect up to two pull requests: one grouping every minor and patch bump, and one grouping the majors - the `lint` job's `checkout` and `setup-go` belong to the latter. Review them like any other change and merge each once the required checks pass.
4. Rollback: delete `.github/dependabot.yml`; open dependency pull requests can be closed, and nothing in the workflow depends on the file.

## Open Questions

- The exact cooldown window (7 days for routine, 30 for majors) is a tunable default: it can be relaxed or tightened after a few cycles without changing the specs, the approach, or the task breakdown.
