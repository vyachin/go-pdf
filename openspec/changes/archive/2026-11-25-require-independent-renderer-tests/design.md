# Design

## Context

See `proposal.md` for the finding and review reference.

`.github/workflows/ci.yml` sets `PDF_REQUIRE_RENDER=1` only in `golden` and filters `go test` to `TestDocumentSnapshots|TestRenderSnapshots|TestProfileCoverage`. `internal/engine/render_independent_test.go` owns `TestIndependentRendererSeesContent`, `TestIndependentRendererColoursAndCoordinates` and `TestIndependentRendererCompositesTransparency`, all using `independentTools`.

RV4-29 in `fourth-review-hardening` owns the image and Poppler pin. This proposal owns test selection and mandatory execution, and must retain that pin when both land.

## Goals / Non-Goals

**Goals:**

Ensure existing independent correctness checks actually gate every pull request.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Extend the golden job's selection or add a separate step there for the entire `TestIndependentRenderer` family, retaining both snapshot layers.

2. Use `go test -json` or an equivalent checked test list to require the expected parent tests to run and pass; missing tools and skips remain failures under `PDF_REQUIRE_RENDER=1`.

3. Retain shuffled unit/race jobs without adding a Poppler installation to them. Coordinate renderer version changes with RV4-29 rather than introducing another pin here.

**Alternative considered:** Installing Poppler in every test job adds redundant work; use the existing tool-equipped required job and verify actual selected executions.

## Risks / Trade-offs

A regex can silently match no test after a rename. Count expected runs or derive the selected set from `go test -list`; gate success must not rely solely on a zero process exit code.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
