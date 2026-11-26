# Tasks

## 0. Behavioral invariant tests before implementation

- [ ] 0.1 Add a deterministic matrix of page heights and one/two spanning cells
  with unique numbered text markers. Compare each cell's complete ordered marker
  sequence with an independently declared expectation across all pages, checking
  every marker occurs exactly once. Verify the current continuation loses markers
  and the completed implementation preserves them; line counts alone do not pass.
- [ ] 0.2 Exercise the same fixtures through PDF output and independent
  `pdftotext` extraction in the required renderer job. Check per-cell text order,
  first/last markers at page boundaries, and repeated-header multiplicity
  separately from body text; account only for documented whitespace collapse and
  Unicode normalization. Verify a missing, duplicated or reordered body marker
  fails even when the page count and render golden match.
- [ ] 0.3 Add the unsupported nested-block continuation case with a following
  paragraph. Verify exactly one source-attributed `layout/overflow` for the cell,
  continued rendering of the following paragraph, and matching pre-serialization
  diagnostics from `Validate` and `Convert` using one shared fixture.

## 1. Continuation state per cell

- [ ] 1.1 Replace the single `spanCell`/`spanX`/`spanW`/`spanRemain`/`spanPage`
  slot in `internal/layout/box.go` with a slice of continuation records (cell, x,
  width, remaining height, covered-row end, content cursor). Record every clipped
  spanning cell in `paintRow` in grid order. Verify with a table that has two
  spanning cells in one row: both records exist and the second does not replace
  the first.
- [ ] 1.2 Consume the records at the top of the next page in `placeTable`, painting
  each cell's fragment and removing it only when its covered rows end. Verify that
  a `rowspan="2"` cell whose rows end mid-page stops its fragment at the last
  covered row and leaves the following rows untouched.
- [ ] 1.3 Continue the cell's content: store the index of the first unpainted line
  and paint the remaining lines at the next fragment's top, advancing the cursor.
  Verify that a cell with `rowspan="100"` and one hundred lines extracts all one
  hundred lines across three pages.

## 2. Decoration and diagnostics

- [ ] 2.1 Paint the fragment's background and all four border sides through
  `decorationOps`, each clipped to the page content box, so every fragment is a
  closed box at its own edges. Verify with a 2 pt red border and a background:
  every covered page contains the fill and strokes on all four sides, and no op
  extends below the page content bottom.
- [ ] 2.2 Report `layout/overflow` exactly once per spanning cell when a fragment
  cannot be produced, using the existing `warnOverflow` latch. Verify a cell that
  exposes no line cursor produces one diagnostic, not one per page, and the rest
  of the document still renders.

## 3. Regression tests and documentation

- [ ] 3.1 Strengthen `TestRowspanCellIsClippedAndContinued` in
  `internal/layout/margin_geometry_test.go`: assert the extracted line count per
  page, the border sides and the background on pages 2 and 3, not only one fill.
  Verify the test fails on the parent commit.
- [ ] 3.2 Add a regression for two spanning cells in one row and one for a span
  ending mid-page. Verify both fail on the parent commit and pass here.
- [ ] 3.3 Update affected render goldens with
  `go test ./internal/engine -run TestRenderSnapshots -update` and review the diff;
  add a `CHANGELOG.md` entry under `### Fixed`. Verify
  `go test -shuffle=on ./internal/layout ./internal/engine` and
  `go test -race ./internal/layout` pass.

## Workflow follow-up

- Run `openspec validate fix-rowspan-page-continuation --strict` and fix any
  reported issue.
- Sync the delta into `openspec/specs/pagination/spec.md` and archive the change
  after review.
