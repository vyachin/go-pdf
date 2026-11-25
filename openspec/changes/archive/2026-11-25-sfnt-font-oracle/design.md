# Design

## Context

`internal/font` parses the table directory, `head`, `hhea`, `maxp`, `loca`, `glyf` (composite glyphs included), a `cmap` subtable of format 4 or 12, and `GPOS` kerning. `Subset` rewrites `head`, `hhea`, `maxp`, `hmtx`, `loca` and `glyf`, keeps `name`, `OS/2`, `cvt `, `fpgm` and `prep`, preserves glyph numbering, and deliberately drops `cmap`.

Today the package is checked against itself: `TestSubset` parses the generated subset with the same `Parse` that wrote it, and `pdfcpu` validates the PDF around the font, not the tables. The committed corpus has fourteen TrueType fonts: the bundled `testdata/fonts/DejaVuSans.ttf` and `DejaVuSans-Bold.ttf`, plus twelve from the weasyprint corpus (FiraSans, SourceSans3, Pacifico, ComingSoon, LibreBarcode and others).

`golang.org/x/image/font/sfnt` is the Go project's independent decoder: `Parse`, `GlyphIndex`, `GlyphAdvance`, `LoadGlyph` returning segments, `Metrics`, `NumGlyphs`, `UnitsPerEm`, and `Kern`. It imports `x/image/font`, `x/image/math/fixed` and `x/text/encoding/charmap`. It has no encoder and no subsetter.

See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:**

- An independent decoder answers the same questions as the private parser: which rune maps to which glyph, how wide a glyph is, where its outline goes.
- The subsetter is validated by a decoder that did not write its output.
- The suite is offline, deterministic, uses no new tool, and has a bounded runtime.
- A real divergence fails; a documented limitation of either side is skipped with a reason, never silently.

**Non-Goals:**

- Kerning comparison.
- Using `sfnt` in production: it cannot subset and does not expose the raw tables the subsetter rewrites; the private parser stays the recorded exception.
- Comparing rasterised glyphs.
- Adding fonts or fixtures: the corpus is already committed.

## Decisions

### 1. A test-only module requirement

`go.mod` gains `golang.org/x/image v0.47.0`. The imports live only in `_test.go` files, so the package's runtime graph is unchanged. `x/image` requires `x/text v0.43`; the module already pins `v0.44`, so MVS keeps the newer one, and `x/sys` is not reached from `sfnt`.

*Alternative*: vendor the needed decoder code — rejected: it re-creates exactly the private implementation this oracle exists to check.

### 2. Corpus discovery with explicit skips

A helper walks `testdata` once per test run, collects `*.ttf` and `*.otf` — the CFF-flavoured files exercise the first skip rule — and parses each file with both decoders. A font is skipped, with a logged reason, when:

- the engine rejects it (a CFF/`OTTO` font, which the profile does not support);
- the engine maps no rune at all while `sfnt` maps `A` — the font's `cmap` format is outside the supported 4 and 12;
- `sfnt` rejects a font the engine accepts — worth triage, but not a reason to fail this suite on the first day.

The test fails when fewer than ten fonts were actually compared. A suite that silently compares nothing is worse than no suite.

### 3. `cmap` over a fixed range

For every compared font, iterate U+0000–U+FFFF and a supplementary sample of U+1F300–U+1F6FF; compare presence and glyph id. The range covers Latin, punctuation, Cyrillic, Greek, Hebrew, Arabic and a CJK sample. Iterating the full Unicode space would multiply the time for no extra coverage; a font whose mapping differs outside the range is caught by the shape-time diagnostics in the engine tests.

### 4. Advances and metrics with a one-unit tolerance

For every glyph id in `[0, NumGlyphs)`, compare the engine's advance in font units with `sfnt.GlyphAdvance` at `ppem = UnitsPerEm` divided by 64. Compare `UnitsPerEm`, `NumGlyphs`, ascent and descent from `sfnt.Metrics` the same way. The tolerance is one unit: the fixed-point conversion rounds, and both decoders read the same `hmtx`.

### 5. Outlines after canonicalising the start point

TrueType stores a contour from an arbitrary start point, and that point may be off-curve; `sfnt` normalises the contour to start at an on-curve point. The test converts the engine's `[]Contour` into the same segment sequence before comparing: start at the first on-curve point, or at the midpoint of the last and first points when both are off-curve; then emit `LineTo` for an on-curve point and `QuadTo` for an off-curve one, with the implied midpoint between two consecutive off-curve points. The segment ops and points must match `sfnt.LoadGlyph`, with a tolerance of one 26.6 unit.

*Why this way*: a raw point comparison would fail on correct fonts because of the start point alone, and a bounding-box comparison would miss a wrong control point. The conversion is a test helper, not a second parser.

*One implementation detail is mirrored rather than interpreted*: when a contour
starts and ends off-curve, `sfnt` emits the closing quad through the last
control point and then does not mark the contour closed, so it closes it a
second time through the first control point. The conversion emits both segments,
because the oracle compares the sequence `sfnt` produces, not an idealised one.

*Coverage and cost*: every glyph of every compared font, composites included, because both decoders expand them. A benchmark guards the runtime; if the corpus grows, the tasks say to sample glyphs per font.

### 6. The subsetter is checked by the independent decoder

`Subset` is called with the glyphs of a fixed Latin and Cyrillic string, and the output is parsed with `sfnt`. Assert that it parses, that `NumGlyphs` is unchanged (numbering is preserved by design), and that every retained glyph keeps its advance and its outline.

*The two tables the subsetter drops and `sfnt` requires*: `sfnt.Parse` calls
`parseCmap` and `parsePost` unconditionally and fails with `invalid cmap table`
or `invalid post table` when they are absent, so the bytes `Subset` produces
cannot be parsed as they are. The test restores the original font's `cmap` and
`post` alongside the subsetter's rewritten `head`, `hhea`, `maxp`, `hmtx`,
`loca` and `glyf`, and lets `sfnt` read those as written. Because the subsetter
preserves glyph numbering, the original `cmap` must still resolve every rune to
the same glyph — and the test asserts exactly that, which is also the proof that
the numbering contract holds.

This is the part that no current test covers: the rewritten directory, the checksums and the `loca`/`glyf` bytes are read by a decoder that did not produce them.

### 7. Kerning is not compared

`sfnt.Kern` reads a Latin-only subset of `GPOS` and models lookups differently; the engine applies several lookups in order and supports class-based subtables. A comparison would report differences that are not defects. The exclusion is stated in the test file and in the `CONTRIBUTING.md` exception note.

### 8. A fuzz target reuses the same corpus

`FuzzParserAgreement` is seeded with the bytes of the corpus fonts. The engine must not panic on any input; when both decoders accept it, `NumGlyphs` and `UnitsPerEm` must agree and a small `cmap` sample must agree. Seeds run in the ordinary test run, so the "no panic" promise gets a second, independent input generator without a fuzzing budget in CI.

## Risks / Trade-offs

- **A legitimate difference fails the suite** → the three skip rules, the one-unit tolerance and the start-point canonicalisation; a failure is triaged before any test is weakened.
- **The suite is slow** → fourteen fonts, linear work; the outline comparison is the expensive part. A benchmark task fixes the budget and sampling is the fallback.
- **The suite compares nothing** → the ten-font minimum.
- **`sfnt` is wrong** → a divergence is a finding: reproduce it minimally, check the bytes against the format, and decide which decoder is wrong. `pdfcpu` strict mode already covers the PDF level.
- **A test-only module widens the graph** → pinned version, no runtime import, covered by the allowlist; CI downloads it once per run.
- **Third-party fonts** → already committed and read-only here; no new licensing surface.

## Migration Plan

None: tests and a test-only requirement. Reverting removes the test file, the helper and the module requirement.
