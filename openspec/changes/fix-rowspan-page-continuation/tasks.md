# Tasks

## 1. Continuation state per cell

- [x] 1.1 Replace the single `spanCell`/`spanX`/`spanW`/`spanRemain`/`spanPage`
  slot in `internal/layout/box.go` with a slice of continuation records (cell, x,
  width, remaining height, covered-row end, content cursor). Record every clipped
  spanning cell in `paintRow` in grid order. Verify with a table that has two
  spanning cells in one row: both records exist and the second does not replace
  the first.
- [x] 1.2 Consume the records at the top of the next page in `placeTable`, painting
  each cell's fragment and removing it only when its covered rows end. Verify that
  a `rowspan="2"` cell whose rows end mid-page stops its fragment at the last
  covered row and leaves the following rows untouched.
- [x] 1.3 Continue the cell's content: store the index of the first unpainted line
  and paint the remaining lines at the next fragment's top, advancing the cursor.
  Verify that a cell with `rowspan="100"` and one hundred lines extracts all one
  hundred lines across three pages.

## 2. Decoration and diagnostics

- [x] 2.1 Paint the fragment's background and all four border sides through
  `decorationOps`, each clipped to the page content box, so every fragment is a
  closed box at its own edges. Verify with a 2 pt red border and a background:
  every covered page contains the fill and strokes on all four sides, and no op
  extends below the page content bottom.
- [x] 2.2 Report `layout/overflow` exactly once per spanning cell when a fragment
  cannot be produced, using the existing `warnOverflow` latch. Verify a cell that
  exposes no line cursor produces one diagnostic, not one per page, and the rest
  of the document still renders.

## 3. Regression tests and documentation

- [x] 3.1 Strengthen `TestRowspanCellIsClippedAndContinued` in
  `internal/layout/margin_geometry_test.go`: assert the extracted line count per
  page, the border sides and the background on pages 2 and 3, not only one fill.
  Verify the test fails on the parent commit.
- [x] 3.2 Add a regression for two spanning cells in one row and one for a span
  ending mid-page. Verify both fail on the parent commit and pass here.
- [x] 3.3 Update affected render goldens with
  `go test ./internal/engine -run TestRenderSnapshots -update` and review the diff;
  add a `CHANGELOG.md` entry under `### Fixed`. Verify
  `go test -shuffle=on ./internal/layout ./internal/engine` and
  `go test -race ./internal/layout` pass.

## Workflow follow-up

- Run `openspec validate fix-rowspan-page-continuation --strict` and fix any
  reported issue.
- Sync the delta into `openspec/specs/pagination/spec.md` and archive the change
  after review.
