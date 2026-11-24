# Design

## Context

See [proposal.md](proposal.md) for motivation and
[specs/golden-tests/spec.md](specs/golden-tests/spec.md) for the requirements.

The current suite freezes three things at corpus level: a whitespace-free glyph
stream extracted from the PDF (`testdata/golden/*.txt`, `testdata/mpdf/golden`),
byte equality of two conversions of the same input, and PDF validity through
pdfcpu. A poppler-based test asserts that a rendered page is neither blank nor
mostly covered, and that one synthetic 200x200pt page has a red block in the
expected corner.

Two helpers promised by the README do not exist: there is no layout snapshot,
and `raster.Diff` (`internal/backend/raster/raster.go`) has no caller.
`layout.Describe` is marked "for snapshots" and is unused too.

Constraints that shape the design:

- The module is dependency-free (no `go.sum`), so no `go-cmp`, no image
  comparison library and no YAML parser. `encoding/xml`, `image/png` and
  `crypto/sha256` are available in the standard library.
- Geometry is integer millipoints, and `css.Length` remembers whether a value is
  absolute (`MP`), a percentage or an em. Absolute lengths are therefore exactly
  representable as points with three decimals.
- `css.Style` is a computed struct of 26 fields; there is no
  "specified value + origin" model, so a snapshot can only show effective
  values.
- `prepare` (`internal/engine/convert.go`) already produces everything the
  snapshot needs and does not load a system font: the fallback font is loaded
  lazily during layout.
- poppler is installed in CI (`PDF_REQUIRE_RENDER=1`) and absent from a default
  developer machine; the existing tests already skip in that case.

## Goals / Non-Goals

**Goals:**

- Freeze what the engine understood (document model) and what it drew (page
  raster) as two independent, reviewable artifacts.
- Fail with enough information to review a difference without rerunning.
- Keep the maintenance loop cheap: one update flag, staleness detection, a
  recorded renderer.
- Make the fixture corpus cover the profile, so a green golden means something.

**Non-Goals:**

- A public API or CLI command for the document snapshot. The format is internal
  so it can evolve without a compatibility promise.
- Snapshots for the mPDF corpus. Its 149 documents keep their text goldens; XML
  and PNG for that volume would dominate the repository.
- Byte-level PDF goldens. Determinism is already asserted by converting twice;
  a committed PDF would additionally pin the Go compression implementation and
  produce unreviewable diffs.
- Replacing the text goldens or pdfcpu. The render layer cannot see the text
  layer, and neither layer validates PDF structure.
- HTML to PNG without a PDF. The render snapshot must go through the writer, or
  it cannot catch a writer regression.

## Decisions

### 1. Two layers, split at the PDF writer

The document snapshot covers everything up to the point where layout starts; the
render snapshot covers everything after it. A regression therefore reports its
half of the pipeline: a failing document snapshot is a parser, cascade or
profile change; a render snapshot that fails while the document snapshot passes
is a layout, pagination, font or writer change.

*Alternatives*: a layout manifest (page ops with coordinates) sits between the
two and gives readable numeric diffs, but it duplicates what the render layer
already reveals and needs a new serializer for a model that is currently
unexported. PDF bytes are unreviewable: a one-glyph change rewrites 98% of the
file because CIDs are allocated in first-use order. Own `raster.Render` produces
PNG without poppler, but it draws from `layout.Ops` and would pass while the PDF
writer is broken.

### 2. XML for the document snapshot

XML reads like the document: an element per HTML element, a style as a set of
attributes. A reviewer can scan it without a decoder, and `encoding/xml` is in
the standard library.

*Alternatives*: JSON would be marginally easier to compare (map keys marshal in
sorted order) but reads as an object tree; canonical HTML would need the same
attributes and would be indistinguishable from XML here. Any of the three is
acceptable; XML was chosen for reviewability, which is the whole point of the
artifact.

The serializer is the only writer of the format, so canonical bytes are part of
the contract: sorted attributes, one element per line, two-space indent, LF,
a `version` attribute on the root. Comparison is byte equality; a formatter
change is a single, intentional golden update.

### 3. Snapshot content: effective values, elided when equal to the parent

Each element is written with the attributes the cascade actually changed, plus
`id`, `class` and its text. An inherited property is elided when it equals the
parent's computed value, because that is what inheritance produces; a
non-inherited property is elided when it equals the initial value, so an inline
element does not repeat the block margins of its parent. Lengths are written as
exact points with up to three decimals (`16000mp` → `16pt`, `17008mp` →
`17.008pt`), colours as `#rrggbb` or `transparent`, keywords as themselves. A
uniform four-sided margin, padding or border is written as the shorthand the
profile supports; a partial set stays in longhand. An unresolved `auto` margin
or padding is written as `0pt`, because layout resolves it to zero, and every
`em` length has already been resolved against the font size by the cascade.

Writing all 26 fields would put about 2300 values into `invoice.xml` and about
16000 into `bigtable.xml`, most of them repeated `font-family` and `font-size`,
and any default change would rewrite every golden. Elision keeps
`invoice.xml` around 250 lines and every line meaningful.

*Trade-off*: an elided value is not visible on the child, so a bug that
inherits a property that should not inherit would show once, on the parent, not
on the child. The alternative - all fields - is exactly what makes the artifact
unreviewable.

The snapshot also carries `<meta>`, `<page>`, `<margin-box>`, `<font-face>` and a
sorted `<diagnostics>` block. Diagnostics must be included: without them a
silently dropped element or attribute is frozen as normal behaviour.

### 4. Styles come from the same walk that layout uses

`docdump` walks the parsed document in document order and calls
`css.Cascade.Compute` with the parent's style, mirroring the traversal in
`layout.Build`. A test asserts, for every element that produces a box, that the
snapshot's style equals the style on the box. This keeps the snapshot honest
without exposing layout internals.

*Alternatives*: dumping the box tree would be a single source of truth, but it
loses `display: none` elements (invisible in the snapshot although the engine
understood them) and introduces anonymous boxes that have no HTML tag. A second
walk without the invariant test is one refactor away from lying.

### 5. Text is normalised, not raw

Text nodes are written with whitespace runs collapsed to one space and trimmed
at block boundaries, and preserved verbatim inside `white-space: pre`. Raw text
would make a reindented fixture change every golden despite an identical
rendering, and it would not verify the normalisation the engine performs.

*Trade-off*: normalisation is re-implemented in the serializer rather than
shared with the flow builder. The difference is small (the flow builder also
does line breaking, which the snapshot must not contain) and the render layer
covers the real flow.

### 6. Render snapshots through poppler at 72 dpi

```
Convert(html) -> PDF -> pdftoppm -png -r 72 -> decode -> raster.Diff(actual, golden, 0)
```

72 dpi keeps a page at 595x842 pixels and a golden around 70-150 KB. Measured on
the invoice fixture: a 1 mm margin change moves **12.41%** of the pixels, while
adding one glyph changes **34 pixels (0.01%)**. Exact comparison is therefore
the only safe rule; a percentage tolerance would hide the second case.

*Alternatives*: 150 dpi doubles the fidelity and quadruples the size, for a
change no test has ever needed; Ghostscript or `mutool` would add a second
dependency for the same information.

`raster.Diff` is reused for the comparison, so its documented purpose becomes
true. Its tolerance is `0`; the pixel count and the bounding box of the
differences are reported.

### 7. The renderer is part of the golden, and is recorded

The rasterizer's version is committed next to the images. In strict CI mode
(`PDF_REQUIRE_RENDER=1`) a mismatch fails with both versions and the regeneration
command; locally the check is skipped. CI pins `ubuntu-24.04` instead of
`ubuntu-latest` so an image bump is a deliberate commit.

*Why not ignore the version*: after a silent poppler upgrade every page differs,
and the failure looks like a layout bug. Naming the renderer turns a confusing
red build into one instruction.

### 8. Render goldens use the bundled font and ignore `PDFTEST_FONT`

Pixels must not depend on the developer machine. The render tests load
`testdata/fonts/DejaVuSans.ttf` directly and skip when it is absent, as the
engine corpus tests already do; the document snapshot needs no font at all.

### 9. Scope of each layer

- Document snapshots: the four corpus documents and every profile fixture
  (small, cheap, and they make the XML the specification of the profile).
- Render snapshots: the four corpus documents (six pages) and the fixtures that
  paint something - table borders and background, margin boxes, an image.
- Text goldens: unchanged, including the mPDF corpus.

This bounds the repository growth to roughly 0.5-1 MB of PNG.

### 10. One update flag, one failure directory

Both layers live in `internal/engine` tests and reuse its existing `-update`
flag. Artifacts of a failure go to `$GOLDEN_ARTIFACTS` when it is set and to
`t.TempDir()` otherwise; CI sets the variable and uploads the directory. Without
a stable directory a failing test loses its images when the test binary exits.

### 11. Coverage is checked by scanning the fixtures

For every entry of `css.PropOrder` and `css.SupportedElements` the coverage test
looks for a declaration or a tag in the committed fixtures. The scan is textual
and can be fooled by a comment, but it catches the real failure mode: a new
property ships with no fixture, and both golden layers stay green for it.

*Alternative*: instrumenting the engine to report which properties each fixture
exercises would be exact, but it puts test bookkeeping into the production
pipeline for a check that runs once per release.

## Risks / Trade-offs

- **[Renderer drift breaks the whole render layer]** → pin the runner image,
  record the version, fail with the regeneration command in strict mode.
- **[Binary goldens cannot be reviewed in a pull request]** → failure artifacts
  (produced, committed, diff) in a stable directory, uploaded by CI, plus a
  documented review procedure in `docs/golden.md`.
- **[The first snapshot freezes current bugs]** → the first generation is
  reviewed by hand; the artifact is designed to make that review possible
  (about 250 lines per document, six images).
- **[The serializer's cascade walk diverges from layout]** → the box/style
  equality invariant test.
- **[Coverage scan gives false confidence]** → documented as a heuristic; the
  real coverage is what the visual set renders.
- **[Repository growth]** → PNG only for six pages and a few visual fixtures;
  XML is small text.
- **[72 dpi misses subpixel differences]** → accepted: the layer caught a
  single added glyph in 34 pixels, which is the smallest change that matters for
  a print document.

## Migration Plan

1. Land the serializer and the document snapshots first; the suite stays green
   and the new tests run everywhere, because they need no external tool.
2. Land the fixture set and the coverage test; adjust the corpus where a profile
   entry is uncovered.
3. Land the render layer with poppler; regenerate its goldens in the same commit
   and record the renderer.
4. Update CI (pinned image, artifact upload) and the README in the same change.
5. Rollback is a revert: no production code path, public API or data format
   changes; the only user-visible effect is a longer `go test ./...`.

## Open Questions

- Whether to promote the document snapshot to a documented CLI command
  (`pdfgen parse`). It would help template authors debug a cascade, but it turns
  the format into a compatibility promise. Deferred until someone asks; the
  serializer is already isolated behind an internal package.
