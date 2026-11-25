# Proposal

## Why

CI installs Poppler only in `golden`, but that job selects only snapshot and profile-coverage tests. `TestIndependentRenderer*` can therefore skip in unit/race jobs and never execute in the one job that owns their tools.

Review reference: **R10** (P2), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Run every independent-renderer test in the tool-equipped required job.
- Require Poppler and fail on missing tools, skipped selected tests or an empty test selection.
- Keep the gate dependent on that job and document which job owns independent checks.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: Require independent text/geometry/color/transparency renderer checks in CI.

## Impact

`.github/workflows/ci.yml` sets `PDF_REQUIRE_RENDER=1` only in `golden` and filters `go test` to `TestDocumentSnapshots|TestRenderSnapshots|TestProfileCoverage`. `internal/engine/render_independent_test.go` owns `TestIndependentRendererSeesContent`, `TestIndependentRendererColoursAndCoordinates` and `TestIndependentRendererCompositesTransparency`, all using `independentTools`.

RV4-29 in `fourth-review-hardening` owns the image and Poppler pin. This proposal owns test selection and mandatory execution, and must retain that pin when both land.

No new public Go identifiers or external dependencies are required.
