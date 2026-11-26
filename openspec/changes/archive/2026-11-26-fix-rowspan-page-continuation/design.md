# Design

## Context

`pagination` already requires a spanning cell that crosses a page break to be
fragmented. The current implementation is a single slot on the table box
(`spanCell`, `spanX`, `spanW`, `spanRemain`, `spanPage` in `internal/layout/box.go`)
filled by `paintRow` while it walks the cells of one row. The next page's
`placeTable` paints one fill for that slot when the background is set, then clears
it. Three consequences follow: a second spanning cell in the same row overwrites
the first; only a background is continued, never text or borders; and after one
continuation the content below the page is gone.

`paintCell` already receives the visible clipped height and knows how to paint
the cell's background, borders and static content for one fragment. Row heights
are computed once before placement (`rowHeights`), so the vertical budget of a
span is stable across pages.

## Goals / Non-Goals

**Goals:**

- Every spanning cell in a row has its own continuation across pages.
- Text, background and all four border sides are painted on every page the span
  covers, clipped to the content box.
- No line is dropped without a `layout/overflow` diagnostic.
- Ordinary tables and non-spanning cells keep byte-identical layout.

**Non-Goals:**

- Recomputing row heights from the continued content.
- Splitting a single line of a cell across pages, or supporting a cell taller than
  the representable page range.
- Changing the grid algorithm, `rowspan`/`colspan` bounds, or the repeated-header
  rules.

## Decisions

**Per-cell state in grid order, not a map keyed by pointer.** The table box keeps a
slice of continuation records (cell, x, width, remaining height, covered-row end,
consumed-content cursor). `paintRow` appends a record when it clips a spanning
cell; `placeTable` consumes records in that slice's order at the top of the next
page. A map would work but its iteration order must never reach the output, and a
slice keeps the existing determinism rule explicit. Records are removed when their
covered rows end, so the slice stays proportional to cells crossing a break.

**Continue text lines, not only a fill.** A text cell's lines are already built by
`buildLines`; the record stores the index of the first line not painted. The next
fragment paints from that index at the fragment's top, and the cursor advances by
the number of lines that fit. A cell whose content is block-level (a nested table,
a list) has no line cursor in this change: its remaining blocks are clipped at the
fragment and reported once, so the loss is visible and the text-line case is
complete.

**Decoration is per fragment, not per cell.** Each fragment paints its own
background and all four border sides at the fragment's edges: left and right run
the full fragment height, and the top and bottom sides close the fragment at the
page boundary, so a continued cell never appears as an unbordered column of text.
The first fragment's top and the last fragment's bottom coincide with the cell's
true edges. The fragment's bottom is clipped to the page content box before it is
painted.

**One diagnostic per cell.** `warnOverflow` reuses its per-box latch (the existing
`overflowWarned` pattern) so a pathological span reports `layout/overflow` once,
not once per page.

## Risks / Trade-offs

- **Double-painted backgrounds on a page boundary** → every fragment is clipped to
  `[fragTop, pageBottom]` and the next starts at the new page's top; a pixel-level
  render golden for a span fixture verifies the boundary.
- **Row heights no longer match the painted cell** → heights stay computed exactly
  as today; a regression test asserts a non-spanning table is unchanged and the
  span's covered rows still end at the table's bottom.
- **Render goldens change** → expected for fixtures with a spanning cell; the
  update is a reviewed commit naming the behaviour that moved.
- **The continuation slice grows on a table with many spanning cells** → records
  exist only while a span crosses a page; the adversarial table fixture that
  bounds cell count still bounds it.
- **A spanning cell with nested blocks keeps only its text-line continuation** →
  the remaining blocks are clipped with one `layout/overflow`; full block
  fragmentation is a follow-up, and no content disappears silently.

## Migration Plan

Internal layout change: no public API, no stored data, no output-format change
beyond the missing content now appearing. Release note under `Fixed`. Existing
documents that never put a spanning cell across a break are unaffected.
