# Design

## Context

`placeBlock` owns page creation for height-driven breaks, but `break-before` creates a
page in `placeChild`, `break-after` is deferred in `breakPending`, and `placeTable`
creates pages for row breaks. `breakPending` is cleared before the next box is
examined. Table measurement runs before `buildGrid`. Two predicates describe "the page
has content": `f.empty` and `len(f.page.Ops) > 0`, and repeated headers paint
operations, so neither distinguishes a header from body content.

## Goals / Non-Goals

**Goals:** one pending-break state machine for blocks and rows; adjacent breaks produce
one transition; measurement and painting use the same grid; no empty pages.

**Non-Goals:** changing `break-inside: avoid` priority, `orphans`/`widows` or repeated
header semantics; changing `MaxPages`; post-processing pages to drop empty ones.

## Decisions

- **D1. One pending break bit for both properties.** `break-before` and `break-after`
  set the same flag; it is consumed when body content is about to be placed. If the page
  already has body content, a new page starts; if not, the flag clears without a
  transition, because the boundary is already a fresh page. This replaces the eager
  `newPage` in `placeChild`, the pending check in `placeBlock` and the row path.
- **D2. Track body content separately.** A `pageHasBody` flag is set when a block with
  content is entered and when a body row is painted; `newPage` clears it. Repeated
  headers do not set it, so a break after a repeated header does not break twice.
  `f.empty` keeps its meaning for orphans and widows.
- **D3. An empty box does not consume the break.** The flag is cleared only by a
  transition or by content that paints; `boxHasContent` includes height, background,
  border, image and children.
- **D4. Rows return control to the page loop.** A row-level break returns a "page
  needed" signal to `placeBlock`, which lets every ancestor finish its fragment before
  the page loop creates the next page.
- **D5. Measurement matches painting.** `measureTable` calls a guarded `buildGrid` once
  and caches it, so `computeColumns` sees the real columns; `buildLines` adds the indent
  once and records it in `Line.Natural`.

## Risks / Trade-offs

- Deferred `break-before` interacts with orphans and widows → the pending break is
  satisfied by the transition those rules take; a test pins it.
- Page counts of adjacent-break documents change → that is the fix; review the diff and
  keep a second golden for the intended old shape if needed.
- The row signal changes the placement return contract → keep a small result struct and
  cover every branch with the pagination suite.
- Regenerating table goldens may hide a regression → review each changed page and keep
  the old fixture where behaviour is intended to change.
