# Tasks

## 1. Margin box shaping failures (LAY-11)

- [x] 1.1 Emit a `text/glyph` error diagnostic naming the margin box when shaping fails — verification: a footer with U+10FFFF fails conversion, no silent omission
- [x] 1.2 Keep ordinary footers working — verification: existing margin box tests pass

## 2. Block images (LAY-13)

- [x] 2.1 Add a branch in `placeBlock` that places a `Kind == Image` box with `imageSize` dimensions and advances the flow — verification: `<p>BEFORE</p><img style="display:block"><p>AFTER</p>` produces an image operation
- [x] 2.2 Paginate an image that does not fit and clip an oversized one with the block overflow rule — verification: image near the page bottom moves to the next page
- [x] 2.3 Keep an image-only document working — verification: a single block image paints once

## 3. Acceptance

- [x] 3.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate silent-loss-fixes` — verification: everything green
