# Proposal

## Why

Three table defects share one root: the table path decides geometry and page
content with narrower or simpler rules than the block path.

- **Columns.** `computeColumns` sums the declared widths in `int32`
  (`used += w`). Two `width:2000000pt` cells wrap the sum; the scale-to-fit pass
  then saturates the total and produces two columns of about 448,795 mp each. The
  second column begins around 505 pt and its box would end around 954 pt on a
  595 pt page.
  The conversion still emits "the table's declared columns are wider than the
  content box and are scaled to fit", which is false, and no cell is reported as
  overflowing.
- **Cell text.** Only `placeText` checks `line.Natural > width`. A thousand-
  character word inside a table cell is painted past the page edge with no
  `layout/overflow`, while the same word in a paragraph is reported.
- **Breaks.** `boxHasContent` returns `len(b.Rows) > 0` for a table before it looks
  at height, background or border. An empty `<table><tr><td></td></tr></table>`
  therefore counts as page body and consumes a pending `break-before`, producing a
  blank page; a table that paints only a height or a background does not count and
  the pending break is consumed by content that should have followed it. The
  same early return exists for an image box.

## What Changes

- Compute the running sum of column widths in 64 bits, scale against the true
  total, and clamp the produced widths so the last column's right edge stays
  inside the content box; keep the single `layout/overflow` warning truthful.
- Move the unbreakable-token check into a helper shared by `placeText` and the
  table cell painter, so a word wider than a cell reports `layout/overflow` once.
- Make `boxHasContent` a paint predicate for every kind: a table counts when a
  row or cell can paint something or its own height/background/border is visible;
  an empty table counts as nothing; a height- or background-only table (or image)
  still consumes a pending break.

Not included: changing the fixed-layout algorithm, `colspan`/`rowspan` bounds, or
the repeated-header rule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: the column-fit requirement gains a 64-bit running sum and a
  truthful post-scale geometry; the unbreakable-token requirement is extended to
  table cells.
- `pagination`: the break-priority requirement gains a paint predicate for table
  and image boxes, so an empty table neither consumes a break nor creates a blank
  page.

## Impact

- `internal/layout/table.go` (`computeColumns`, `paintCell`/`paintStaticBlock`),
  `internal/layout/paginate.go` (`boxHasContent`, `placeText`), and the overflow
  helper in `internal/layout/paint.go`.
- Tests: `internal/layout/margin_geometry_test.go`,
  `internal/layout/pagination_test.go`; a render golden for the two-column
  fixture and the empty-table break fixture may change.
- No public API, profile or diagnostic-code change. `layout/overflow` already
  exists.
