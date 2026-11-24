# Tasks

## 1. Document snapshot serializer

- [x] 1.1 Add `internal/docdump` with a single entry point that takes the
  prepared document model (document, cascade, page setup, diagnostics) and
  returns `[]byte`. The root element carries `version="1"`; attributes are
  sorted, elements are one per line with two-space indentation and LF endings.
  Verify with a package test that a two-element document produces the expected
  bytes and that a second serialization of the same model is byte identical.
- [x] 1.2 Implement effective-style elision: walk the document in document order
  with the parent style and write only what the cascade changed. An inherited
  property is elided when it equals the parent's computed value; every other
  property is elided when it equals the initial value (`css.DefaultStyle`).
  Write a uniform four-sided margin, padding or border as the shorthand; a
  partial set stays in longhand. Render absolute lengths as points with up to
  three decimals (`16000` -> `16pt`), percentages as `%`, an unresolved `auto`
  margin or padding as `0pt` (layout resolves it to zero), colours as `#rrggbb`
  or `transparent`, keywords verbatim; `em` is already resolved by the cascade.
  Verify with tests for inherited versus changed `font-size`, `margin` and
  `color`, for the shorthand compression, and for the exact point rendering of a
  millipoint value that is not a whole point.
- [x] 1.3 Normalise text nodes: collapse whitespace runs to one space, trim at
  block boundaries, and preserve text verbatim inside a `white-space: pre`
  element. Verify with a test that reindenting the source fixture does not
  change the snapshot and a test that a `pre` block keeps its spacing.
- [x] 1.4 Emit the document-level blocks: `<meta>` from `<title>` and meta tags,
  `<page>` with the resolved size and margins in points, `<margin-box>` for each
  margin box with its content items (leaving `counter(page)`,
  `counter(pages)` and `target-counter(...)` as tokens), `<font-face>` for each
  declaration, and `<diagnostics>` sorted by position with code, severity,
  position and element path. Verify against `testdata/invoice.html`: the
  snapshot contains 210mm x 297mm as points, both margin boxes invoice declares
  (`top-right` and `bottom-center`) and no resolved page numbers.
- [x] 1.5 Add the consistency invariant: for every element that produces a box
  from `layout.Build`, the computed style in the snapshot equals the style on
  the box. Verify the test passes on the four corpus documents; confirm it
  fails when the serializer is given the parent's style instead of the element's
  style for one fixture, then restore.

## 2. Document goldens and profile fixtures

- [x] 2.1 Add `TestDocumentSnapshots` to `internal/engine`, reusing the
  package's `-update` flag. The fixture set is the four corpus documents plus
  every `testdata/fixtures/*.html`; snapshots live in `testdata/golden/xml/`.
  A fixture without a snapshot fails and names the regeneration command; a
  snapshot without a fixture fails as stale. On a mismatch, report the first
  differing lines with line numbers and the produced snapshot path. Verify by
  running the test, deleting one golden (fails, names the command), adding an
  orphan golden (fails as stale), and restoring both.
- [x] 2.2 Generate and commit the four corpus snapshots. Review them by hand:
  `invoice.xml` shows `font-size="11pt"` on `body`, `font-size="16pt"` and
  `margin-bottom="17.008pt"` on `h1`, `color="#666666"` on the muted paragraph,
  and a collapsed-border table; `contract.xml` shows a centred heading and an
  ordered list; `bigtable.xml` stays under ~3000 lines. Verify the test passes
  without `-update` and `du -sh testdata/golden/xml` is under 200 KB.
- [x] 2.3 Add small fixtures under `testdata/fixtures/` for the profile
  entries the corpus does not reach: `break-before`, `break-after`,
  `break-inside`, `orphans`, `widows`, a `target-counter` table of contents,
  `@font-face` with the bundled font, `rowspan`, `tfoot`, `hr`, `br`, `ul`,
  `h4`-`h6`, `strong`/`em`/`a`, and an `<img>` with a committed 16x16 PNG. Keep
  each fixture under 40 lines. Verify every fixture converts with
  `go run ./cmd/pdfgen validate testdata/fixtures/<name>.html` (exit 0, no
  warnings for the supported ones).
- [x] 2.4 Add the coverage test: fail when a name from `css.PropOrder` or
  `css.SupportedElements` does not appear in the snapshot fixture set
  (`testdata/*.html`, `testdata/fixtures/*.html` and their stylesheets).
  Verify it passes on the committed set, then remove the `widows` declaration
  from its fixture and confirm the failure names `widows`; restore it.

## 3. Render snapshots

- [x] 3.1 Add a render helper used by the tests: convert a fixture, write the
  PDF to a temporary directory, run `pdftoppm -png -r 72` without `-f` or
  `-singlefile` so every page is produced, and decode all pages with
  `image/png`. The helper loads `testdata/fonts/DejaVuSans.ttf` directly,
  ignoring `PDFTEST_FONT`, and skips when the font or poppler is unavailable.
  Verify with a test on `invoice.html` that one page of 595x842 is produced and
  that the pixel at the page corner is white.
- [x] 3.2 Add `TestRenderSnapshots` with the `-update` flag. Compare every page
  with `raster.Diff(actual, golden, 0)`; on a mismatch report the page number,
  the number of differing pixels and their bounding box, and write the produced
  image, the committed image and a difference image into `$GOLDEN_ARTIFACTS`
  when the variable is set, otherwise into `t.TempDir()`. Missing and stale
  goldens fail exactly as in 2.1. Verify by running with `-update`, then without
  it, then changing a fixture margin from 6mm to 5mm and confirming the failure
  reports a non-zero count, a bounding box and three files.
- [x] 3.3 Record the rasterizer: on update, write the first line of
  `pdftoppm -v` to `testdata/golden/render/renderer.txt`. When
  `PDF_REQUIRE_RENDER=1` and the recorded line differs from the current one,
  fail with both versions and the regeneration command. Verify by editing the
  recorded line: the strict run fails naming both values, `-update` restores it.
- [x] 3.4 Generate and commit the render goldens for the four corpus documents
  and the fixtures that paint: the table fixture (borders and background), the
  margin-box fixture and the image fixture. Review that each page looks like the
  document it came from. Verify the tests pass without `-update` and
  `du -sh testdata/golden/render` is between 400 KB and 1 MB.

## 4. CI

- [x] 4.1 In `.github/workflows/ci.yml`, pin `runs-on: ubuntu-24.04`, set
  `GOLDEN_ARTIFACTS` on the test and race steps, add a `Golden snapshots` step
  that runs both layers explicitly with `-count=1 -v` and fails when any of them
  is skipped, and add an `actions/upload-artifact@v4` step that uploads the
  directory when the job fails. Verify the file parses as YAML, the step passes
  in the pinned image, an induced mismatch exits non-zero and leaves the three
  images plus the produced XML in the configured directory, and a machine
  without the rasterizer fails the skip guard.
- [x] 4.2 Confirm the lint job is untouched and that `PDF_REQUIRE_RENDER=1` and
  `PDF_REQUIRE_VALIDATOR=1` are set for both the test and the race step, so the
  render layer cannot silently skip in CI. Verify with a grep of the workflow
  and by reading the job steps.

## 5. Documentation

- [x] 5.1 Add `docs/golden.md`: the two layers and what each one can and cannot
  catch, the XML format and the elision convention, the update commands, the
  renderer pinning rule and the review procedure for a changed snapshot.
  Verify the documented commands run as written.
- [x] 5.2 Fix the README testing section: replace "Layout snapshots check
  geometry, not pixels" with the document snapshot, make the `raster.Diff`
  sentence name the render layer, add the two commands and note that render
  goldens need poppler and the recorded version. Verify no sentence in the
  section describes a test that does not exist.
- [x] 5.3 Add a line to `docs/mpdf-corpus.md` stating that the mPDF corpus keeps
  text goldens and is outside both snapshot layers, with the reason (volume).
  Verify the statement matches the fixture sets in `internal/engine`.

## 6. Integration verification

- [x] 6.1 From a clean tree run `gofmt -l $(git ls-files '*.go')` (empty),
  `go vet ./...`, `go test ./...` and `go test -race ./...` with
  `PDF_REQUIRE_VALIDATOR=1` and `PDF_REQUIRE_RENDER=1`; confirm from `-v` output
  that the new layers ran rather than skipped.
- [x] 6.2 Confirm the diff contains only `internal/docdump`, `internal/engine`
  tests, `testdata`, `.github/workflows/ci.yml` and documentation: `go.mod`
  unchanged, no `go.sum`, no public API change in `pdfgen`.
- [x] 6.3 Replay the failure review path once: introduce and revert a 1 mm
  margin change, confirm the document snapshot reports the changed line and the
  render layer reports the pixel count, bounding box and three images, and
  finish with `git status` clean.

## Workflow follow-up

- After merge, mark the `test` job as a required check in branch protection:
  the workflow runs the snapshots on every push and pull request, but only the
  repository setting prevents a red build from being merged.
- After merge, archive this change and confirm `openspec list --specs` shows the
  new `golden-tests` capability with its purpose.
