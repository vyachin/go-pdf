# Proposal

## Why

A cell that spans rows does not reserve the height it needs. `rowHeight` divides the
cell's measured height by the span, but that dividend is credited only to the row that
owns the cell; the covered rows below keep only their own cells' heights, so the sum of
the rows a cell spans can be smaller than the cell. Layout then paints the cell's
background and borders over that sum and paints the cell content without clipping, so
the content runs past the table and the next block is placed on top of it with no
diagnostic. The reviewer's reproduction puts the last line of the spanning cell and the
paragraph after the table at the same Y. The existing tests only cover spans whose
neighbouring rows are already tall enough, so nothing catches the deficit.

## What Changes

- The sum of the heights of the rows a cell spans SHALL be at least the cell's outer
  height; when it is smaller, layout MUST grow the covered rows by the deficit so the
  content stays inside the cell and the following block starts after it.
- One shared row-height computation SHALL feed table measurement, pagination and the
  static path used for tables nested in cells, so the three call sites cannot disagree.
- Tests SHALL cover a tall spanning cell next to short or empty covered rows with a block
  after the table, and the measurement path used by `break-inside: avoid`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: the rows covered by a `rowspan` cell cover its height, so cell
  content never overlaps the block that follows the table.

## Impact

- **Code**: `internal/layout/table.go` (one row-height helper used by `measureTable`,
  `placeTable` and `paintStaticTable`), layout and pagination tests.
- **Output**: documents whose spanning cells already fit keep their geometry; documents
  with a deficit gain height and may paginate differently, which is the correction.
- **Risk**: table geometry feeds page breaks, so the mpDF corpus and the `rowspan`
  golden are the regression gate; the existing `rowspan.html` fixture fits today and is
  expected to stay byte-identical.
