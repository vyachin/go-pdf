# Tasks

## 1. Column geometry

- [ ] 1.1 In `internal/layout/table.go`, accumulate the declared column widths in
  `int64` and scale against the true total. Verify with two `width:2000000pt`
  cells that each scaled width is positive and their sum is at most the content
  width.
- [ ] 1.2 Clamp the produced widths with an `int64` running total so the last
  column's right edge is at most the content box's right edge. Verify the cell
  boxes for both wide columns end inside the page.
- [ ] 1.3 Emit the existing `layout/overflow` warning only after the clamp, at most
  once per table. Verify the two-column fixture reports a warning whose message is
  true of the produced geometry, and a fitting table reports none.
- [ ] 1.4 Add a regression test with two 2,000,000 pt columns that asserts the
  right edge of each cell against the content box. Verify the test fails on the
  parent commit.

## 2. Cell text overflow

- [ ] 2.1 Extract `checkLineOverflow(b *Box, line Line, width int32)` from
  `placeText` in `internal/layout/paginate.go`, using `int64` comparison and the
  per-box `warnOverflow` latch. Verify the block-level long-word test still passes.
- [ ] 2.2 Call the helper from the cell text painter in `internal/layout/table.go`
  with the cell's content width. Verify `<table><tr><td>` plus a thousand-character
  word reports one `layout/overflow`, and a short word reports none. Both tests
  must fail on the parent commit for the long word.

## 3. Break content predicate

- [ ] 3.1 Rewrite `boxHasContent` in `internal/layout/paginate.go`: remove the
  `Table` and `Image` early returns, add the own-height/background/border check for
  every kind, and recurse into table rows and cells, stopping at the first painted
  element. Verify an empty table answers false and a `height:50pt;background:#f00`
  table answers true.
- [ ] 3.2 Add the two break regressions: empty table after a paragraph with
  `break-before:page` yields one page; a height/background-only table after
  `break-after:page` lands on the next page with the following paragraph. Verify
  the page count and per-page ops fail on the parent commit.
- [ ] 3.3 Run `go test -shuffle=on ./internal/layout ./internal/engine`, update any
  affected render goldens with `-update` and review the diff, and add a
  `CHANGELOG.md` entry under `### Fixed`.

## Workflow follow-up

- Run `openspec validate fix-table-layout-defects --strict` and fix any reported
  issue.
- Sync both deltas into `openspec/specs/document-layout/spec.md` and
  `openspec/specs/pagination/spec.md`, then archive the change after review.
