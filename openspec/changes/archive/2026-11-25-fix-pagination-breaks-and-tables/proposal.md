# Proposal

## Why

Pagination has three places that decide to start a page — `placeChild` for
`break-before`, `placeBlock` for a pending `break-after`, and `placeTable` for rows —
so a `break-after` followed by a `break-before` produces two transitions and a blank
page between them. The pending `break-after` is also cleared before the next box is
known to paint anything, so an empty or height-only `<div>` cancels the break, while a
`break-before` on an empty trailing box creates a blank page. A table with
`break-inside: avoid` is measured before its grid exists, so it splits; a forced break
on a `<tr>` creates a page inside `placeTable`, leaving the table background painted
after its cells and with the previous page's coordinates; `text-indent` with an inline
image corrupts the line's natural width; and one tall block reports `layout/overflow`
twice.

## What Changes

- One pending-break decision SHALL serve blocks and table rows: adjacent forced breaks
  collapse into exactly one transition, and a break request MUST NOT create a second
  page when the current page has no body content.
- A pending break SHALL be consumed only by content that paints something; an empty box
  MUST NOT cancel it and MUST NOT create a page.
- `break-inside: avoid` SHALL use the table's built grid for measurement.
- A row-level forced break SHALL return control to the page loop so every ancestor
  fragment completes before the new page is painted.
- `text-indent` SHALL count toward the line's natural width when an inline image
  precedes the first word.
- One overflow diagnostic SHALL be reported per box.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pagination`: forced breaks are decided once from painted body content; adjacent
  breaks collapse; row-level breaks keep ancestor fragments consistent.
- `document-layout`: table measurement uses the built grid; a line's natural width
  includes its indent and images.

## Impact

- **Code**: `internal/layout/paginate.go` (single pending-break state, `pageHasBody`),
  `table.go`, `box.go`, `lines.go`, `paint.go`.
- **Output**: documents with adjacent forced breaks lose the spurious page, which
  changes page counts and `counter(pages)` for those documents — that is the
  correction. The `breaks` golden has no adjacent pair and should stay identical.
- **Tests**: the adjacent-pair, empty-box, trailing-break, table-grid, row-break,
  indent and duplicate-warning reproductions become regressions; repeat headers and
  orphans/widows are covered by the existing suite.
