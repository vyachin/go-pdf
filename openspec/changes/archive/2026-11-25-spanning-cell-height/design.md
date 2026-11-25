# Design

## Context

A row's height is the maximum measured height of the cells that start in it, plus the
declared row `height` as a minimum. A cell with `rowspan > 1` contributes
`cellHeight / rowspan` to the row that owns it; the covered rows below contribute only
their own cells. The sum of the covered rows can therefore be smaller than the cell.

The painted height of a spanning cell is the sum of the covered rows: the background and
borders use that sum, while the cell's content is painted from the top without clipping.
When the sum is short, the content overruns the table and the next block is placed on top
of it. An empty covered row makes the deficit as large as half the cell.

Row heights are computed in three places: `measureTable`, `placeTable` (which caches them
in the table box) and `paintStaticTable` for tables nested in cells.

`document-layout` supports `rowspan` within a single page, so a spanning cell that would
cross a page break stays out of scope.

## Goals / Non-Goals

**Goals:**

- The covered rows always cover the spanning cell's outer height.
- One computation serves measurement, pagination and the static cell path, so they cannot
  disagree.
- A span that already fits keeps today's geometry.

**Non-Goals:**

- Browser-exact proportional distribution of the required height.
- Splitting a spanning cell across pages, or warning about a span that crosses a break.
- Changing `colspan` behaviour.

## Decisions

### D1. Two passes in one row-height helper

`rowHeights(t)`:

1. Base pass: for each row, the maximum of its cells' measured heights, with the existing
   `cellHeight / rowspan` as a floor for the owner row, and the declared row height as a
   minimum. Keeping the floor preserves today's geometry for spans that fit.
2. Deficit pass: for each spanning cell, compute the required outer height; if the sum of
   its covered rows is smaller, add the deficit to the last covered row.

Growing the last covered row keeps the cell content anchored at the top and leaves rows
that already fit untouched, which minimises golden churn and keeps the break point as
close as possible to today's. Alternatives considered:

- Distribute the deficit evenly: closer to what browsers approximate, but it changes row
  boundaries for spans that already fit and makes pagination less predictable.
- Put the whole cell height on the owner row and let the other rows be empty: destroys the
  heights of unrelated cells in the covered rows.
- Clip the cell content and warn: silently loses content, which is the failure mode the
  engine exists to prevent.

### D2. One helper, three callers

`measureTable`, `placeTable` and `paintStaticTable` call the same helper; `placeTable`
caches the result in the table box as today. This removes the three loops that could
drift apart when the distribution rule changes.

### D3. Measurement and placement use the same two passes

The helper calls `l.measure(cell, width)` exactly as today, so line building and the
`cell.width` / `cell.x` side effects happen once per invocation. A test asserts that the
measured table height equals the sum of the placed row heights for a table with a
deficit.

### D4. Growth can move a page break, and that is correct

The table becomes taller, so it may no longer fit where it did. The cell is still painted
only on the row that owns it; a span crossing a page break remains unsupported and is out
of scope. No new split behaviour is introduced.

## Risks / Trade-offs

- [Golden output moves for documents with a deficit] → `rowspan.html` already fits and is
  expected to stay identical; any other diff in the mpDF corpus is reviewed and explained.
- [A taller table moves to the next page] → the spec requires the cell to be covered; tests
  pin the page count.
- [The last-row choice can look unbalanced with `vertical-align: middle`] → the content is
  centred in the covered sum, which is the intended appearance; the even distribution is
  the documented alternative if this is rejected in review.
- [Measurement builds cell lines more than once] → already true today; `bigtable` is the
  benchmark guard.

## Migration Plan

None; internal layout only. Rollback is a revert of the helper and its three callers.
