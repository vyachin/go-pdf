# Proposal

## Why

Four independent layout paths still paint or place content outside the page, or
lose it, without a diagnostic:

- **Empty anchors.** `splitInline` returns no box for an inline element with no
  content, and `buildLines` drops a line that carries only anchors. Verified:
  `<p><span id="empty"></span></p>`, `<p>text <a id="x"></a></p>` and
  `<p><span id="split"></span>tail</p>` all leave the anchor map empty, so a
  `target-counter()` reference resolves to 0 and the engine reports the misleading
  `pagination/unknown-target` for an id that exists.
- **Justified page endings.** `placeText` passes `last` only for the paragraph's
  final line, so every page's closing line is stretched to the content edge.
  Verified: an eighty-word justified paragraph on 200×90 pt pages ends pages 1, 2
  and 3 flush right; only the true final line stays unstretched.
- **Table bottom decoration.** `finishFragment` adds the table's bottom padding and
  border to the fragment bottom and paints them without comparing with the page.
  Verified: a 100 pt page with `padding-bottom:200pt` on a table yields a blue fill
  of 220,070 mp on a 100,000 mp page, with no diagnostic.
- **Margin boxes.** `paintMarginBoxes` subtracts the page margins in `int32` and
  adds positions without saturation. Verified with
  `@page{size:200pt 200pt;margin:10pt 2000000pt}` and
  `@bottom-center{content:"FOOT"}`: the wrapped content width moves the footer to
  an off-page coordinate (about X = -2.1 billion millipoints), and no diagnostic
  names the margin box.

`pagination` and `document-layout` already require geometry to saturate and the
page edges to be respected; these four call sites were never converted.

## What Changes

- Keep an empty inline element with an `id` as a zero-content inline box, and
  register a pending anchor on a zero-height carrier line so an empty anchor keeps
  the page it occupies. An anchor-only line MUST NOT count as page body.
- Pass an end-of-page flag to `paintLineAt` and suppress justification for the last
  line painted on a page, not only for the paragraph's final line.
- Clip a completed table's bottom padding and border to the page content bottom and
  report `layout/overflow` once when the clip happened.
- Compute margin box width and positions with `satAdd`/`satSub`, clamp them into
  the page, and report a diagnostic naming the box when its content cannot be
  placed.

Not included: changing justification rules for `text-align: right`/`center`,
redesigning the anchor model, or supporting margins that leave no content box.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: the inline and anchor requirements gain a rule for empty
  `id` elements; the alignment requirement gains "the page's last line is not
  justified".
- `pagination`: the table-decoration and margin-box requirements gain clipping,
  saturation and a diagnostic instead of off-page paint.

## Impact

- `internal/layout/box.go` (`splitInline`), `internal/layout/lines.go`
  (`buildLines`, anchor carrier), `internal/layout/paginate.go` (`placeText`),
  `internal/layout/paint.go` (`paintLineAt`, `finishFragment`),
  `internal/layout/margin.go` (`paintMarginBoxes`).
- Tests: `internal/layout/pagination_test.go`,
  `internal/layout/margin_geometry_test.go`; render goldens for a justified
  multi-page fixture and a table with bottom padding may change.
- No public API, profile or diagnostic-code change. `layout/overflow` already
  exists.
