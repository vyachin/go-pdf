# Proposal

## Why

Every external input of the build is pinned on purpose, but nothing keeps those pins fresh. `ci.yml` references four distinct actions, and they have already drifted: the `test` job uses `actions/checkout@v7` and `actions/setup-go@v7` while the `lint` job still uses `@v4` and `@v5` of the same actions, so two jobs exercise different runtimes for no stated reason. A pin nobody revisits is indistinguishable from a pin nobody knows how to update.

The module itself has no requirements and no `go.sum`, so the usual reason to add Dependabot — module dependencies — does not apply. Its `github-actions` ecosystem does: it is the only mechanism available here that watches `uses:` refs and opens a reviewable pull request. The other pins are deliberately different and must stay manual: the runner image and poppler determine the golden pixels, `pdfcpu@v0.11.0` is coupled to the module's `go 1.24` minimum, and the linter version is a documented quality pin. Automatic bumping must therefore be scoped to action refs, not introduced as a general policy.

## What Changes

- Add `.github/dependabot.yml`:
  - `github-actions` for `/`, weekly, with a cooldown so a fresh major release does not land in CI on release day;
  - minor and patch bumps grouped into one pull request, majors in their own pull request;
  - a `gomod` entry that stays dormant while the module has no requirements and starts reporting with the first one.
- Do not add auto-merge: no `dependabot/fetch-metadata` workflow and no auto-merge enablement. A dependency pull request is merged only after the required checks pass and a human reviews it.
- Keep the coupled pins manual and out of automation's reach: the `ubuntu-24.04` runner image and its poppler, `pdfcpu@v0.11.0`, the `golangci-lint` version input, and the `go` directive in `go.mod`.
- Make action-version consistency a rule rather than an accident: both jobs SHALL reference the same major of the same action.
- Document the policy in `README.md` next to the existing CI description.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: adds requirements for automated action reference updates, for the boundary between automated and manual pins, and for a single action major across CI jobs.

## Impact

- **CI**: new `.github/dependabot.yml`; `.github/workflows/ci.yml` aligns the `lint` job's `checkout` and `setup-go` with the `test` job - by hand in this change, because the first Dependabot run can only happen after the merge to the default branch.
- **Documentation**: `README.md` gains one paragraph on the update policy.
- **Repository settings**: auto-merge stays off for dependency pull requests. Branch protection is unchanged: `test` and `lint` are already expected to be required checks.
- **Product code**: none. `go.mod` stays dependency-free and no `go.sum` is introduced by this change.
- **Assumption**: the README paragraph is in scope; the question was left open in the exploration and the recommendation was adopted.
