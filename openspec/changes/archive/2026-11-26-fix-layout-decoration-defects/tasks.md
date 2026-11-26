# Tasks

## 1. Empty inline anchors

- [x] 1.1 In `internal/layout/box.go`, make `buildElement` return one empty
  `Inline` box when an inline element has no flow and its node has an `id`, so
  `collectPieces` registers it as an anchor piece. Verify
  `<p>text <a id="x"></a></p>` and `<p><span id="split"></span>tail</p>` populate
  the anchor map on the current build's parent commit.
- [x] 1.2 In `internal/layout/lines.go`, flush an anchor-only carrier line
  (`Height: 0`, `Ascent: 0`, anchors set) when `pendingAnchors` is non-empty and the
  line has no items. In `internal/layout/paginate.go`, make `placeText` register
  such a line's anchors at the current `f.y`, advance by zero and NOT set
  `pageHasBody`. Verify `<p><span id="empty"></span></p>` resolves and the page
  count and visible line count are unchanged.
- [x] 1.3 Add a regression: a two-page document with an empty `id` on page 2 and a
  `target-counter` reference shows page 2 with no `pagination/unknown-target`.
  Verify the test fails on the parent commit.

## 2. Page-ending justification

- [x] 2.1 In `internal/layout/paginate.go`, compute
  `endOfPage := k == fits-1 && b.cursor.line < len(b.lines)-1` in the `placeText`
  loop and pass `last || endOfPage` to `paintLineAt`; rename the parameter to
  `unstretched` in `internal/layout/paint.go`. Verify an eighty-word justified
  paragraph on 200×90 pt pages ends every page short of the content edge.
- [x] 2.2 Keep the table cell painter's `i == len(lines)-1` rule and the
  `ForcedBreak` suppression unchanged. Verify `text-align: right`/`center` output
  is byte-identical and a justified single-page paragraph still stretches all but
  its final line.
- [x] 2.3 Add the three-page justification regression and update the affected
  render golden with `-update`; review the diff before committing.

## 3. Page-boundary geometry

- [x] 3.1 In `internal/layout/paint.go`, clamp a completed table fragment's
  decoration bottom to `f.bottom` in `finishFragment`, leave `afterBlock`'s flow
  advance unchanged, and call `warnOverflow` once via the `overflowWarned` latch.
  Verify the 100 pt page with `padding-bottom:200pt` paints no op past the page and
  reports exactly one `layout/overflow`.
- [x] 3.2 In `internal/layout/margin.go`, saturate and clamp `contentWidth`, each
  `x` and `y` in `paintMarginBoxes`; report one diagnostic naming the box when the
  label cannot be placed. Verify the 200 pt page with `margin:10pt 2000000pt` keeps
  the footer inside the page or diagnoses `bottom-center`, with no negative
  coordinate.
- [x] 3.3 Verify ordinary margin boxes and ordinary table decoration are
  byte-identical; run `go test -shuffle=on ./internal/layout ./internal/engine` and
  `go test -race ./internal/layout`; add a `CHANGELOG.md` entry under `### Fixed`.

## Workflow follow-up

- Run `openspec validate fix-layout-decoration-defects --strict` and fix any
  reported issue.
- Sync both deltas into `openspec/specs/document-layout/spec.md` and
  `openspec/specs/pagination/spec.md`, then archive the change after review.
