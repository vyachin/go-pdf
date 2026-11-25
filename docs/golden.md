# Golden snapshots

The suite freezes the pipeline in two places. Both are ordinary Go tests with a
`-update` flag; there is no external snapshot tool.

```
HTML -- parse/cascade --> document snapshot (XML)   testdata/golden/xml/
     |
     v  layout, pagination, PDF writer
   PDF -- poppler --> page snapshots (PNG)           testdata/golden/render/
```

A failure in the first layer is a change in parsing, the cascade or the
profile. A failure in the second layer, with the first one green, is a change in
layout, pagination, font handling or the PDF writer.

## Document snapshots

`TestDocumentSnapshots` writes one XML file per fixture to
`testdata/golden/xml/`. The fixture set is the four corpus documents and every
`testdata/fixtures/*.html`.

The snapshot is the document model the converter prepares before layout:
elements, their effective styles, `@page` setup, margin boxes, `@font-face`
declarations, metadata from the head, and the sorted diagnostics. It needs no
font, no PDF and no rasterizer, so it runs on every machine.

Format rules:

- `<?xml version="1.0" encoding="UTF-8"?>` and a root `<document version="1">`;
  the version changes only when the serializer output changes.
- Attributes are sorted, one element per line, two-space indentation, LF.
  Comparison is byte equality.
- An **inherited** property is omitted when it equals the parent's computed
  value. Every other property is omitted when it equals its initial value. A
  missing attribute therefore means "inherited from the parent" or "initial".
- A uniform four-sided `margin`, `padding` and `border` is written as the
  shorthand the profile supports. A partial set stays in longhand
  (`margin-bottom`, `border-left`, ...), because a one-value shorthand would set
  the other sides too.
- Lengths are exact points with up to three decimals: 6mm is `17.008pt`.
  An unresolved `auto` margin, padding or text indent is written as `0pt`,
  because layout resolves it to zero. An `em` length is already resolved against
  the font size by the cascade.
- Colours are `#rrggbb`, `transparent`, or `unset` when nothing declared one.
- Text is normalised: whitespace runs collapse to one space, and a leading or
  trailing space survives only when it separates inline content. Inside
  `white-space: pre` the text is verbatim.
- `counter(page)`, `counter(pages)` and `target-counter(...)` stay as tokens:
  their values are decided by layout.
- The head appears only as `<meta title="..." author="..."/>`; `<style>`,
  `<link>`, `<title>` and other metadata elements are not dumped as content.

## Render snapshots

`TestRenderSnapshots` converts the visual fixture set and rasterizes every page
with `pdftoppm -png -r 72`. The set is the four corpus documents and every
`testdata/fixtures/visual-*.html`.

- Comparison is pixel exact (`raster.Diff(actual, golden, 0)`). A measured one
  millimetre margin change moves about 12% of the pixels, while one added glyph
  moves 34 pixels, so a percentage tolerance would hide real regressions.
- `visual-hello-world` is the cover fixture: the only page painted edge to edge
  from a photograph, with `background-image` and `background-size: cover`. Its
  golden is the acceptance test for background painting, cover cropping and the
  clip, so a change there is reviewed like any other golden.
- A failure reports the page number, the number of differing pixels and the
  bounding box, and writes three images to `$GOLDEN_ARTIFACTS` (or the test's
  temporary directory): `produced`, `expected` and a red-on-gray `diff`.
- The renders are produced with the bundled `testdata/fonts/DejaVuSans.ttf`.
  `PDFTEST_FONT` is ignored here on purpose: pixels must not depend on the
  machine.
- `testdata/golden/render/renderer.txt` records the `pdftoppm -v` line the
  goldens were produced with. CI (`PDF_REQUIRE_RENDER=1`) fails when the
  rasterizer differs; locally the layer is skipped instead, because a renderer
  difference would otherwise look like a layout regression.

The committed goldens belong to `pdftoppm` 26.01.0 from Ubuntu 26.04, which is
the release `ubuntu-latest` serves today and the version `renderer.txt` records.
The label floats, so the pixel images must be regenerated on the new image
(`go test ./internal/engine -run TestRenderSnapshots -update` inside it) whenever
the label rolls; the workflow, this document, `CONTRIBUTING.md` and
`renderer.txt` then name the same image and the same poppler in one change.

## Commands

```bash
# Check both layers
go test ./internal/engine -run 'TestDocumentSnapshots|TestRenderSnapshots'

# Regenerate after an intended change
go test ./internal/engine -run TestDocumentSnapshots -update
go test ./internal/engine -run TestRenderSnapshots -update

# A filtered run does not report the other goldens as stale
go test ./internal/engine -run TestDocumentSnapshots/invoice
```

`TestProfileCoverage` fails when a property from `css.PropOrder` or an element
from `css.SupportedElements` is not declared by any snapshot fixture
(`testdata/*.html`, `testdata/fixtures/*.html` and their stylesheets).
The scan is textual and can be fooled by a comment, but it catches the real
failure mode: a feature ships and both snapshot layers stay green because no
fixture ever runs it.

## Adding a fixture

1. Put a small document under `testdata/fixtures/`. It must validate without
   warnings: `go test ./internal/engine -run TestDocumentSnapshots` fails with
   "fixture is not valid" while it does not.
2. Regenerate the document snapshot.
3. Name it `visual-<topic>.html` if its purpose is painting; then also
   regenerate the render snapshots.
4. Review the new golden: it is the contract, so a mistake committed now is
   frozen. Remove a fixture and both its goldens together.

## Reviewing a change

- A document snapshot diff is the review: the changed line names the element and
  the property.
- A render snapshot diff is a review artifact, not a test output. Open the three
  images from `$GOLDEN_ARTIFACTS` and compare them, then either fix the change
  or regenerate the golden. CI uploads them as the `golden-artifacts` artifact
  when the job fails.
- Never update a golden to make a failing build green without deciding that the
  new output is correct.

## Continuous integration

Both layers run in the `golden` job of `.github/workflows/ci.yml`, which runs on
`ubuntu-latest` and installs poppler and pdfcpu. The same job runs the
independent renderer checks (`TestIndependentRenderer*`), which compare the PDF
against another renderer's text, colours and transparency; the job lists their
expected names, so a rename that drops one from the selection fails there instead
of passing with an empty run. The label floats: the job follows
the image GitHub serves, so when that image rolls the renderer changes and the
render goldens are regenerated in the change that accepts the new rasters. In addition, a dedicated
`Golden snapshots` step runs the three tests explicitly with `-count=1 -v` and
fails when any of them is skipped:

```
if grep -q -- '--- SKIP' golden.log; then exit 1; fi
grep -q -- '--- PASS: TestDocumentSnapshots ' golden.log
grep -q -- '--- PASS: TestRenderSnapshots ' golden.log
```

The suite already contains the tests, but a suite can be green because a test
was removed, renamed or skipped. The step turns "the snapshots exist" into
"the snapshots ran": a missing rasterizer fails through `PDF_REQUIRE_RENDER=1`,
and a future skip fails through the grep. The step also sets
`GOLDEN_ARTIFACTS`, so a failure uploads `produced`, `expected` and `diff`
images as the `golden-artifacts` artifact.

Branch protection requires the `gate` job — which depends on every test job and
fails when one of them fails or is skipped — not the individual jobs, and that is
a repository setting rather than a workflow file: without it a red build can
still be merged. The `lint` job is separate and does not run the snapshots.

## What these layers do not cover

- PDF structure: `pdfcpu` validation, as before.
- The text layer (copy and search): the extract-based text goldens in
  `testdata/golden/*.txt` and `testdata/mpdf/golden/`.
- The mPDF corpus: it keeps text goldens only, because 149 documents would make
  the snapshot directories dominate the repository.
