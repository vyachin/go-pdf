# Design

## Context

Four paint paths do arithmetic or bookkeeping that the rest of layout already does
safely:

- `splitInline` builds fragments only from the flow of an inline element. An
  element with no content yields an empty flow and therefore no box, so its `id`
  never reaches `collectPieces`. When a line ends with pending anchors and no
  items, `buildLines`' `flush(false)` returns early and the anchors are lost.
- `paintLineAt(page, b, line, x, y, width, last)` decides justification from
  `last`, and `placeText` sets `last := b.cursor.line == len(b.lines)-1`. A line
  followed by a page break is not the paragraph's last line, so it is stretched.
- `finishFragment` sets `bottom = f.y + padBottom + borderBottom` for a completed
  block and calls `decorationOps` without comparing with `f.bottom`. `afterBlock`
  then advances the flow by the same insets.
- `paintMarginBoxes` computes `contentWidth := page.Size.Width - marginLeft -
  marginRight` and `x = contentLeft + (contentWidth-run.Width)/2` (or
  `... - run.Width`) in plain `int32`.

## Goals / Non-Goals

**Goals:**

- An empty inline `id` keeps the page it occupies; a reference resolves without
  `pagination/unknown-target`.
- The last line painted on a page keeps its natural width under
  `text-align: justify`.
- A table's bottom decoration never paints past the MediaBox; the clip is
  reported once.
- Margin box coordinates saturate and stay on the page; an unplaceable box is
  diagnosed, not silently moved off-page.
- Documents that do not hit these paths keep byte-identical output.

**Non-Goals:**

- Anchoring an empty element to a precise line inside a block that splits across
  pages (it anchors at the first line it can attach to, or the block's start).
- Changing `text-align: right`/`center`, or the `target-counter` grammar.
- Allowing a page margin that leaves no content box.

## Decisions

**Empty inline elements produce an empty inline box.** In `buildElement`, when an
inline element's flow is empty and the node has an `id`, return one `Inline` box
with no children. `collectPieces` then registers its `id` as an anchor piece at the
exact point in the inline flow, which is what `<span id="x"></span>tail` needs.

**A zero-height carrier line holds anchors that have no content after them.**
`buildLines` flushes an anchor-only line (`Height: 0`, `Ascent: 0`) when
`pendingAnchors` is non-empty and no items were collected. `fitMetrics` leaves a
zero-ascent line at zero height, `countFitting` counts it as fitting, and
`placeText` registers its anchors at the current `f.y` and advances by zero. Such a
line MUST NOT set `pageHasBody`, so it cannot satisfy a pending break by itself.

**Justification receives an end-of-page flag.** `placeText` computes
`endOfPage := k == fits-1 && b.cursor.line < len(b.lines)-1` and passes
`last || endOfPage` to `paintLineAt`. The parameter is renamed to `unstretched` to
avoid implying paragraph-final. The table cell painter keeps passing `i ==
len(lines)-1` while a cell is painted on one page; when the `rowspan`
continuation from `fix-rowspan-page-continuation` paints a fragment, it passes
the flag for that fragment's last line, so a continued cell obeys the same rule.

**Clip the table decoration, warn from the box.** In `finishFragment`, when a
completed fragment's bottom exceeds `f.bottom`, the decoration bottom is clamped to
`f.bottom` and `warnOverflow(b, "...")` is called once through the existing
`overflowWarned` latch. The flow still advances by the declared insets in
`afterBlock`, so the following block starts on the next page; only the paint is
clipped.

**Saturate and clamp margin box geometry.** `contentWidth` becomes
`max(0, satSub(width, satAdd(left, right)))`; each `x` is `satAdd` and then
clamped to `[0, pageWidth-run.Width]` (or `0` when the run is wider than the page);
`y` is saturated and clamped into `[0, pageHeight]`. When the clamp or a
non-positive content width prevents placing the label, the box reports
`layout/overflow` once naming the margin box, and the label is still painted at the
clamped position when one exists.

## Risks / Trade-offs

- **An empty anchor adds a zero-height line to a paragraph** → it paints nothing
  and does not count as page body; a test asserts the page count and the visible
  line count are unchanged for a paragraph with an empty anchor.
- **A justified page-ending line is shorter** → expected; render goldens for the
  justified fixture change and are reviewed with `-update`.
- **A clipped table bottom border is drawn at the page edge** → the alternative
  is paint outside the MediaBox; the diagnostic records the decision.
- **A margin box with a huge margin may still be unplaceable** → the run is painted
  at X=0 when it fits the page, otherwise diagnosed; either way no wrapped
  coordinate reaches the content stream.

## Migration Plan

Internal layout change: no public API, no stored data. Anchor resolution improves
for documents that referenced empty ids (previously `pagination/unknown-target`,
now the real page). Page-ending justification and extreme table/margin geometry
change only where the defect applied. Release notes under `Fixed`.
