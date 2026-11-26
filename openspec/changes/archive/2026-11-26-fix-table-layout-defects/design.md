# Design

## Context

Tables use a fixed layout: `computeColumns` takes the widths of the first row,
divides the remaining space between the columns that have no width, and scales the
result when the declared total exceeds the content width. `paintCell` paints the
cell's background, borders and text without asking whether a line fits, because
only `placeText` asks that. `boxHasContent`, used by the break state machine, has
per-kind early returns; for a table it answers from the row count alone.

The three defects were reproduced on the current build: two 2,000,000 pt columns
produce a false "scaled to fit" warning and cell boxes past the page; a
thousand-character word in a cell produces no diagnostic; `<p>A</p>` followed by
an empty table with `break-before:page` produces a second, blank page, while a
height/background-only table after `break-after:page` stays on page 1.

## Goals / Non-Goals

**Goals:**

- After scaling, the sum of column widths is at most the content width and every
  cell box is inside the page.
- A `layout/overflow` warning, when present, describes the geometry that was
  actually produced, and is emitted at most once per table or cell.
- A word that overflows a cell is reported exactly as the same word in a block.
- An empty table does not consume a break and does not create a page; a table that
  paints a height or a background does consume it.
- Ordinary tables keep byte-identical output.

**Non-Goals:**

- Supporting a `break-inside: avoid` table taller than the page.
- Changing row-height computation, header repetition, or the fixed-layout
  distribution rule when the widths fit.
- Recomputing a cell's content width from its spans.

## Decisions

**Scale in `int64`, then clamp the running sum.** The declared total is summed in
`int64`; if it exceeds `available`, each width becomes
`int64(w) * int64(available) / total`. The assignment then walks the columns with
an `int64` running total and clamps each width to `available - used`, giving the
remainder to the last column. This makes "the sum is at most the content width" a
loop invariant rather than a hope, and it keeps the rounding deterministic.

**Warn from the produced geometry.** The warning stays `layout/overflow` and the
existing message, but it is emitted only after the clamp and only when the declared
width asked for more than the box; the message says the columns were scaled, which
is then true. A table whose widths already fit emits nothing. The check is per
table (`overflowWarned` on the table box).

**One overflow predicate for blocks and cells.** Extract
`checkLineOverflow(b *Box, line Line, width int32)`, which compares
`line.Natural` with `width` in `int64`, calls `warnOverflow` once per box and
returns whether the line was reported. `placeText` and the cell text painter call
it; the cell painter passes the cell's content width. This also fixes the
duplicate-warning risk when a cell is measured and then painted.

**A paint predicate, not a structural one.** `boxHasContent` no longer returns
early for `Table` or `Image`. A box has content when a text box has non-space text,
a list item always does (the marker), an image has data, or the box's own height,
background, `Background` image or a visible border would paint. A table recurses
into its rows and cells with the same predicate and stops at the first positive
answer. An empty table therefore reports false; a table with `height:50pt;
background:#f00` reports true. The predicate is bounded by the box tree, which the
DOM depth limit already bounds, and it is only called at a break decision.

## Risks / Trade-offs

- **A blank page disappears from an existing document** → this is the defect; the
  page count and `counter(pages)` change for such a document, and the change is
  recorded in the changelog.
- **A height/background-only table moves to the next page** → correct per the
  break contract; a document that relied on the old placement changes one page
  boundary.
- **Column widths differ by one millipoint from the previous overflow case** →
  only for tables that already overflowed; the clamp is deterministic and the
  render golden records it.
- **The predicate walks a large table** → it stops at the first painted cell and
  runs once per break decision, not per row paint.

## Migration Plan

Internal layout change: no public API, no stored data. Page counts change only for
documents that hit the empty-table break defect, and column geometry changes only
for tables that already exceeded the content width. Release notes under `Fixed`.
