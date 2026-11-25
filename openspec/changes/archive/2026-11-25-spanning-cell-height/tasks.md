# Tasks

## 1. Row height

- [x] 1.1 Add one `rowHeights(t)` helper with a base pass (measured cells, owner-row `cellHeight/rowspan` floor, declared row height) and a deficit pass that grows the last covered row until the sum covers a spanning cell — verification: a unit test with a tall `rowspan` cell and an empty covered row asserts that the sum of the covered rows is at least the measured cell height
- [x] 1.2 Replace the three row-height loops in `measureTable`, `placeTable` and `paintStaticTable` with the helper — verification: a test asserts the measured table height equals the sum of the placed row heights for a table with a deficit
- [x] 1.3 Add the end-to-end reproduction: a table whose first-row `rowspan="2"` cell contains more lines than the covered rows need, followed by a paragraph with `margin: 0` — verification: the paragraph's baseline is below the last line of the counting cell, no overflow diagnostic is emitted, and the test fails before the fix

## 2. Regression and corpus

- [x] 2.1 Pin that a span which already fits keeps its geometry, using the existing `rowspan` fixture — verification: `TestRowspanCoversSeveralRows`, `TestRowspanIsClampedToRemainingRows`, `TestRowspanShiftsFollowingCells` and the golden document snapshot are unchanged
- [x] 2.2 Re-run the mpDF corpus and review every document whose page count changes — verification: `go test ./...` and the corpus pass; each diff is explained in the change verification
- [x] 2.3 Add an Unreleased/Fixed entry to `CHANGELOG.md` describing the covered-height rule — verification: the entry is present and matches the behaviour pinned by the tests

## 3. Acceptance

- [x] 3.1 Run `go vet ./...`, `go test ./...`, `go test -race ./...` and `openspec validate spanning-cell-height` — verification: all green
