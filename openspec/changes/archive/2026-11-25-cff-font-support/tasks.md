# Tasks

## 1. Spike: the corpus fonts through sfnt

- [x] 1.1 Parse `testdata/weasyprint/ticket/barlowcondensed-{regular,bold,light}.otf` with `sfnt`: record the glyph count, the `cmap` and `GPOS` presence, the number of segments per glyph and the advance range — check: all three parse, no CFF2 or variable font among them, and the numbers are recorded in the change or the design (recorded in `design.md`, which also records that `sfnt` cannot read `flex1` and so cannot supply the outlines)
- [x] 1.2 Convert a handful of glyphs by hand and render them through the existing raster to confirm the orientation and the fixed-point scale before writing the encoder — check: the converted `А` and `g` produce the same shape as the original at a coarse raster

## 2. The CFF reader

- [x] 2.1 `internal/font/cff.go`: parse the `CFF ` table — header, the Name, Top DICT, String and Global Subr INDEX structures, the Private DICT, CharStrings, and `FDArray`/`FDSelect` for a CID-keyed font — check: the three Barlow fonts yield 695 charstrings, every glyph the font maps resolves to a readable charstring, and a malformed INDEX or a truncated table is rejected without a panic
- [x] 2.2 `internal/font/cff_charstring.go`: a Type 2 interpreter producing cubic segments in 26.6 font units — numbers, the moveto/lineto/curveto families, `flex`/`hflex`/`flex1`/`hflex1`, the hint operators with `hintmask` operand skipping, `callsubr`/`callgsubr`/`return` with the ten-level limit, the arithmetic and stack operators, and a fixed `random` — check: per glyph, the segments equal `sfnt`'s wherever `sfnt` loads the glyph, and hand-computed `flex`/`flex1` cases match to the unit
- [x] 2.3 Bound the interpreter and name what it refuses: an instruction budget, the subr depth and the per-glyph point cap; a cyclic charstring or a `seac` `endchar` fails with an error naming the glyph — check: three tests, and no panic on a fuzzed charstring

## 3. The converter

- [x] 3.1 Add `font.Load`: dispatch on the sfnt version, keep `font.Parse` TrueType-only, and reject WOFF/WOFF2, CFF2 and variable fonts with a diagnostic naming the format — check: the existing `TestRejectCFF` on `Parse` stays meaningful; `Load` accepts the three Barlow files
- [x] 3.2 Convert CFF segments to quadratic contours with integer de Casteljau subdivision at the midpoint until the quadratic is within 16 fixed-point units of the cubic, with a per-glyph cap of 8192 points — check: unit tests on a straight cubic, a circular arc and a glyph from Barlow; every point within the tolerance
- [x] 3.3 Encode `glyf` (simple glyph flags, short and long vectors) and `loca` (long format), rewrite `maxp` as version 1.0 with the real maxima, patch `head` and assemble the font with `buildSFNT` — check: the converted bytes parse with `font.Parse` and `sfnt`, and `pdfcpu validate` accepts a PDF that embeds the converted font (task 5.1); pdfcpu has no standalone font checker
- [x] 3.4 Copy `cmap`, `GPOS`, `name`, `OS/2`, `post`, `hhea` and `hmtx` unchanged and prove the numbering is preserved — check: `GlyphIndex`, `Advance` and `Kerning` agree between the original CFF (through `sfnt`) and the converted font for the corpus
- [x] 3.5 Prove determinism: converting the same bytes twice yields identical output — check: a byte comparison in the test

## 4. Bounds and diagnostics

- [x] 4.1 Refuse a converted `glyf` larger than four times the input (and more than 64 MiB) or a glyph beyond the point cap, with `resource/limit` naming the font and the glyph — check: three tests, one per bound
- [x] 4.2 Keep the corrupt-font path unchanged: a truncated CFF fails with the existing parse error, not a panic — check: a truncated fixture and a fuzz seed

## 5. End-to-end

- [x] 5.1 Convert a document that declares a Barlow OTF, assert the PDF embeds the used glyphs and the text layer extracts the document text — check: `pdfdebug` extraction and `pdfcpu` strict mode
- [x] 5.2 Verify the subset: the embedded font contains the used glyphs, their outlines match the converted font and the unused slots are empty — check: the existing subset test extended to the converted font
- [x] 5.3 Freeze a render golden for the OTF document — check: two runs byte identical, then a run without `-update`
- [x] 5.4 Reject WOFF/WOFF2 and a remote font with the format named — check: three scenarios from the spec

## 6. Documentation and acceptance

- [x] 6.1 `docs/fonts.md`: OpenType/CFF is accepted through conversion; the 16-unit fit and the one-unit stored outline, the point cap and the four-times bound; `seac` is refused; what `fsType` does not check — check: the numbers match the constants
- [x] 6.2 `docs/profile.md` and the README font section: `.otf` is accepted — check: the example declares an OTF and converts
- [x] 6.3 CHANGELOG: the capability under Added, the lack of a runtime dependency under Changed — check: the Unreleased section
- [x] 6.4 `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` — check: green
- [x] 6.5 The pinned `golangci-lint` run and `openspec validate cff-font-support --strict` — check: green and valid
