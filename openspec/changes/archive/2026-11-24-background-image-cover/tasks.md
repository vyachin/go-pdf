# Tasks

## 1. Profile surface and the fixture that declares it

- [x] 1.1 Add `background-image` (a `url()` reference) and `background-size` (`cover` only) to `css.PropOrder`, `css.Table`, `css.Style` and the parsers in `internal/css`; verify `go test ./internal/css` covers the accepted declarations and rejects a gradient, a remote URL, `contain` and a length pair.
- [x] 1.2 Serialise both properties in `internal/docdump` (`background-image` as the source string, `background-size` as the keyword); verify a focused `docdump` test shows the attributes on the element that declares them and on no other.
- [x] 1.3 Copy the photograph to `testdata/fixtures/hello-world.jpg` without re-encoding and verify `shasum -a 256` matches the file in the repository root.
- [x] 1.4 Write `testdata/fixtures/visual-hello-world.html` per the design: A4, zero margins, a 210 mm table with one 297 mm cell, `padding: 0`, `vertical-align: middle`, `text-align: center`, the background image, and `h1` at 84 pt/700 in `#1c2b3a` reading "Hello world"; verify `go run ./cmd/pdfgen validate -base-dir testdata/fixtures -font-dir testdata/fonts testdata/fixtures/visual-hello-world.html` exits 0 with no diagnostics.
- [x] 1.5 Regenerate the document snapshot with `go test ./internal/engine -run TestDocumentSnapshots/visual-hello-world -update`, review the new `testdata/golden/xml/visual-hello-world.xml` line by line, and verify `go test ./internal/engine -run 'TestDocumentSnapshots|TestProfileCoverage'` passes.
- [x] 1.6 Document both properties in the table and the "What is missing" notes of `docs/profile.md`, including the centered-cover semantics and the `@page` exclusion; verify the table row matches the values `go run ./cmd/pdfgen validate -print-profile` reports.

## 2. Layout: cover geometry and painting

- [x] 2.1 Add the cover geometry helper (scale to cover, split the overflow equally on opposite edges) in `internal/layout`; verify table-driven tests cover a 2:3 image on A4, an image wider than the box and an image taller than the box, with exact offsets.
- [x] 2.2 Emit the background image op before the box's content from `decorationOps`, with a clip rectangle equal to the painted fragment; verify a layout test asserts the image op precedes the text op and that its rectangle and clip are the box fragment.
- [x] 2.3 Resolve the source through the existing `imageLoader` so the background obeys `BaseDir` confinement, the byte and pixel budgets and the cache; verify an engine test covers a missing file, a path outside the base directory and a remote URL, each as a diagnostic rather than a panic.
- [x] 2.4 Verify `go test ./internal/layout ./internal/engine -run 'TestDocumentSnapshots|Test'` stays green for the existing fixtures, including the image fixture.

## 3. PDF writer: a clip for image operations

- [x] 3.1 Give `ImageOp` an optional clip rectangle and make the writer wrap only that draw in `q <rect> re W n ... Q`; verify a writer test inspects the content stream for the clip and for the unchanged draw when the clip is nil.
- [x] 3.2 Verify `go test ./internal/pdf` passes and that a converted cover page opens with the image inside the page box, not spilling past it.

## 4. Cover fixture end to end and its goldens

- [x] 4.1 Convert the fixture with `go run ./cmd/pdfgen convert -o /tmp/cover.pdf -render-png`; verify zero diagnostics, exactly one page, and that `/tmp/cover.png` shows the photograph edge to edge with "Hello world" centered on both axes.
- [x] 4.2 Verify the produced PDF with `pdfcpu validate -strict` (skip with a note if pdfcpu is unavailable); if the progressive JPEG is rejected, re-encode the same 4175x6263 pixels as a baseline JPEG, repeat the check and record the re-encode in the fixture's HTML comment.
- [x] 4.3 Generate `testdata/golden/render/visual-hello-world-1.png` inside the `ubuntu:24.04` container with poppler 24.02.0; verify `testdata/golden/render/renderer.txt` is unchanged and that `PDF_REQUIRE_RENDER=1 go test ./internal/engine -run TestRenderSnapshots` passes for the new fixture and the existing ones.
- [x] 4.4 Review the committed render golden: 595x842 pixels, no white edge, centered text, the same crop the design describes. A wrong golden now is frozen forever, so this review is the acceptance gate.
- [x] 4.5 Add `visual-hello-world` to the visual set described in `docs/golden.md` and to the feature list in `README.md`; verify the commands quoted in both files run as written.

## 5. Integration

- [x] 5.1 Verify `go vet ./...` and `go test ./... -count=1` pass, including the golden layers when `PDF_REQUIRE_VALIDATOR=1` is available.
- [x] 5.2 Verify `go test -race ./... -count=1` passes, since the image loader is shared by `<img>` and background painting.
- [x] 5.3 Reproduce the CI golden step in the `ubuntu:24.04` container (`PDF_REQUIRE_RENDER=1`, the three golden tests with `-count=1 -v`, and the `grep` for `--- PASS`); verify no test is skipped and the renderer check accepts `renderer.txt`.

## Workflow follow-up

- Archive the change after review, per the project's archive workflow.
- Verify the archived specs and the new goldens are present and that the change directory no longer reports open artifacts.
