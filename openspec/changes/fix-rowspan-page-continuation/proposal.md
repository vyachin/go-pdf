# Proposal

## Why

A `rowspan` cell that crosses a page break is supposed to continue on the next
page, but layout keeps one continuation slot for the whole table. When two cells
span in the same row, the later one overwrites the earlier one; after the first
continuation the slot is cleared again. Verified on the current build: a
first-row cell with `rowspan="100"` and one hundred lines produces three pages,
paints 61 lines on page 1 and none on pages 2 and 3, and reports no diagnostic.
The cell's background and border also disappear after page 1. A table is the
standard way to print invoice line items and totals, so losing lines changes the
document's meaning.

The existing `pagination` requirement already says the cell must be fragmented,
but the implementation only continues a background fill once. This change makes
the requirement true for every spanning cell and for all of its paint.

## What Changes

- Replace the single `spanCell`/`spanRemain` slot with per-cell continuation
  state keyed by the spanning cell, so every `rowspan` cell in a row continues
  independently and in grid order.
- Continue the cell's text, background and all four border sides on every page the
  span covers, clipped to that page's content box.
- Track a consumed-content cursor per spanning cell so lines that did not fit on
  the previous page are the ones painted next, with no gap and no repetition.
- When a fragment cannot be produced, report `layout/overflow` exactly once for
  the cell instead of dropping the remaining content.

Not included: changing the grid algorithm or row-height computation. Only the
fragment after the first is affected.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pagination`: the spanning-cell requirement gains per-cell continuation state,
  per-page background and border painting, and an explicit no-silent-loss
  outcome.

## Impact

- `internal/layout/box.go` (table continuation state), `internal/layout/table.go`
  (`placeTable`, `paintRow`, `paintCell`), `internal/layout/paginate.go`,
  `internal/layout/paint.go`.
- `internal/layout/pagination_test.go` and `internal/layout/margin_geometry_test.go`:
  the existing spanning-cell test asserts only one fill and misses the lost text
  and the missing borders.
- Render goldens for fixtures with a spanning cell may change, because the
  continued fragment now carries text and borders.
- No public API, profile or diagnostic-code change. `layout/overflow` already
  exists.
