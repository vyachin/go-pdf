# Design

## Context

`text-shaping` limits fonts to TrueType, and `font.Parse` rejects the `OTTO` sfnt version with "CFF/OpenType outlines are not supported, use a TrueType font". The PDF writer emits a Type0 font with a `CIDFontType2` descendant, `FontFile2` and `CIDToGIDMap` (`pdf-output` requires exactly that chain), and the subsetter rewrites `head`, `hhea`, `maxp`, `hmtx`, `loca` and `glyf` while dropping `cmap`. Layout and the writer carry `*font.Font` through their APIs, and shaping asks it for `GlyphIndex`, `Advance` and `Kerning`.

`golang.org/x/image/font/sfnt` parses the `CFF ` table and answers `GlyphIndex`, `GlyphAdvance` and `Kern` for every glyph of the corpus fonts, and `sfnt-font-oracle` already uses it as a test-only oracle. What it cannot do is read every outline: its Type 2 interpreter stops at `flex` and `flex1` (see the spike below), which the corpus fonts use in ordinary characters. The engine therefore reads CFF outlines itself. The corpus already contains three CFF fonts (`testdata/weasyprint/ticket/barlowcondensed-{regular,bold,light}.otf`), so no fixture has to be added.

See `proposal.md` for motivation.

## Spike results

Measured on `testdata/weasyprint/ticket/barlowcondensed-{regular,bold,light}.otf` with
`golang.org/x/image/font/sfnt` v0.47.0, at `ppem = UnitsPerEm` (1 outline unit = 1 font unit).

| | regular | bold | light |
|---|---|---|---|
| file size | 138 336 | 141 516 | 138 724 |
| sfnt version | `OTTO` | `OTTO` | `OTTO` |
| units per em | 1000 | 1000 | 1000 |
| glyphs | 695 | 695 | 695 |
| runes mapped by `cmap` | 524 | 524 | 524 |
| tables | `CFF `, `cmap`, `GPOS`, `GSUB`, `GDEF`, `OS/2`, `head`, `hhea`, `hmtx`, `maxp`, `name`, `post`, `DSIG` | same | same |
| CFF2 / `fvar` | absent | absent | absent |
| segments per glyph | 0–73, mean 28.5 | 0–66, mean 28.6 | 0–74, mean 28.5 |
| curve segments | cubic only | cubic only | cubic only |
| empty glyphs | 5 | 5 | 5 |
| advance range (font units) | 0–1022 | 0–1027 | 0–1021 |
| glyphs `LoadGlyph` cannot read | 16 (10 mapped) | 33 (20 mapped) | 30 (21 mapped) |

All three parse, are name-keyed CFF 1 fonts and are not variable. There is no CFF2 to reject among
them, and `cmap` and `GPOS` are present in all three.

**The spike also found a blocker that the context above does not account for.** `sfnt`'s Type 2
charstring interpreter implements `hflex` (12 36) and `hflex1` (12 38) but neither `flex` (12 35)
nor `flex1` (12 37). `LoadGlyph` therefore fails with `sfnt: unrecognized CFF 2-byte operator (12
37)` on 33 glyphs of the bold face, 30 of the light face and 16 of the regular one. Among the
failing glyphs are ordinary text characters: `8` in Barlow Condensed Bold and `r` in Barlow
Condensed Light. `GlyphIndex`, `GlyphAdvance` and `Kern` work for every glyph; only outlines fail.

The newest published release, v0.48.0, has the same gap, so a version bump does not fix it.
Relying on `sfnt` for outlines would mean rejecting or emptying a font because one ordinary glyph
uses `flex1`, and the engine must not ship a font it has silently emptied. The decision recorded
below is that the engine reads CFF outlines itself and keeps `sfnt` as the test oracle.

## Goals / Non-Goals

**Goals:**

- An OpenType/CFF font works end to end: load, shape, subset, embed, extract text.
- The PDF object structure and the subsetter stay as they are; `pdf-output` is untouched.
- Conversion is deterministic across platforms and bounded in work and memory.

**Non-Goals:**

- Native `CIDFontType0`/`FontFile3` embedding. A non-CID-keyed CFF is not a valid `CIDFontType0` program without being rewritten as CID-keyed, which is a CFF subsetter in disguise.
- CFF2, variable fonts and WOFF/WOFF2, and `seac` accent composition.
- A CFF subsetter: the converted font is subsetted by the existing TrueType path.
- An `fsType` embedding-permission check: the engine does not check it for TrueType either, and it is a separate decision.

## Decisions

### 1. Convert CFF to TrueType in memory, preserving glyph IDs

`font.Load(data)` dispatches on the sfnt version: `0x00010000`/`true` goes to the existing parser,
`OTTO` goes to a converter that builds a TrueType font and then hands the bytes to the same parser,
and `wOFF`/`wOF2` are refused. Everything downstream — shaping, the font registry, subsetting,
CIDs, `ToUnicode`, the `CIDFontType2` chain — is unchanged.

The synthetic font keeps the source tables whose contents are glyph-ID independent or stay valid
because the numbering is preserved: `cmap`, `GPOS`, `name`, `OS/2`, `post`, `hhea`, `hmtx`. It
synthesizes `glyf` and `loca`, rewrites `maxp` as version 1.0 with the real maximum points and
contours, and patches `head` (`indexToLocFormat = 1`, the recomputed bbox, `checkSumAdjustment = 0`;
`buildSFNT` recomputes the adjustment). `post` is copied because OpenType names it as required for
TrueType outlines and it stays valid while the numbering does.

*Why this and not a native CFF font object*: the PDF writer would need a new
Type0/`CIDFontType0`/`FontFile3` branch, and a name-keyed CFF would have to be rewritten as
CID-keyed first. Converting to the one format the rest of the engine already handles is less new
surface, and `pdf-output`'s object chain stays true.

*Cost*: the converted font is larger than the CFF original — quadratic outlines and a `glyf` table
that duplicates data — but the subsetter immediately reduces it to the used glyphs.

### 2. The engine reads CFF outlines itself

The `CFF ` table is read in `internal/font`: the header, the INDEX structures (Name, Top DICT,
String, Global Subrs), the Top DICT (CharStrings, Private, and `FDArray`/`FDSelect` for a CID-keyed
font, where the glyph's own font dict supplies the local subrs), the Private DICT (`Subrs`,
`nominalWidthX`, `defaultWidthX`) and the CharStrings INDEX. A Type 2 charstring interpreter turns
one charstring into cubic segments in 26.6 font units: the path operators, the flex family
(`flex`, `hflex`, `flex1`, `hflex1`), the hint operators (parsed only far enough to consume a
`hintmask`/`cntrmask` data), `callsubr`/`callgsubr` with the spec's ten-level limit, and the
arithmetic and stack operators. `random` returns a fixed half so output stays deterministic.
`endchar` carrying the four `seac` operands is refused: composing a glyph from StandardEncoding and
the charset is a separate feature, and an approximate accent is worse than a diagnostic.

Executing a charstring is bounded: an instruction budget and the per-glyph point cap stop a cyclic
or pathological program instead of letting it run.

`sfnt` stays the test oracle and is not imported by production code.

*Why not depend on `sfnt`*: it cannot read `flex` or `flex1`, which the corpus fonts use in ordinary
characters (the spike above). The alternative — a patched fork of `sfnt` — vendors a module to add
two operators, and the project's dependency rule prefers the reader it controls.

### 3. Outline conversion is integer-only

The reader reports coordinates in 26.6 fixed point, so one font unit is 64 fixed-point units.
Cubics are split by de Casteljau at the midpoint, recursively, until the quadratic that passes
through the split point stays within **16 fixed-point units (one quarter of a font unit)** of the
cubic. With the quadratic that passes through the cubic's midpoint the gap between the two curves is
|3(P1−P2)+(P3−P0)| / (12√3), so the integer test is |3(P1−P2)+(P3−P0)|² ≤ 432·tol². All arithmetic
is integer; no float64 enters the result.

*Why integer*: the project promises byte-identical output, and the golden tests compare across
platforms. A float pipeline can differ where a compiler fuses a multiply-add, and a single changed
coordinate changes the PDF. Integer subdivision is deterministic everywhere by construction.

*Tolerance and storage*: the 16-unit figure is the fit between the two curves. `glyf` stores integer
font units, so every point is then rounded to the nearest unit; a stored outline is within **one
font unit** (0.001 em at 1000 font units per em) of the source curve. `docs/fonts.md` records both
numbers, and the tests check both: the fit in fixed point and the stored points against the source.

A glyph that would exceed the per-glyph point cap (8192) fails with `resource/limit` naming the
glyph, rather than growing without bound.

### 4. The converted font is bounded

`MaxFontBytes` bounds the input. The converter additionally refuses to build a `glyf` larger than
**four times the input** (and never more than 64 MiB) or a single glyph beyond **8192 points**;
either failure is `resource/limit` with the font path and the stage. The constants are documented in
`docs/fonts.md` next to the tolerance.

*Why a separate bound*: a small CFF can describe a very dense outline, and `MaxFontBytes` says
nothing about the output size.

### 5. The loader, not the parser, owns the conversion

`font.Parse` keeps its contract: it parses TrueType and rejects `OTTO` (the existing test that a CFF
font is rejected stays meaningful). `font.Load` is the new entry point used by the engine's font
registry and by `Options.FallbackFont`: it inspects the sfnt version, converts when needed and calls
`Parse`. This keeps the parser honest and makes the conversion a separately testable step.

*Alternative*: convert inside `Parse` — rejected: `Parse` would no longer be a parser, and its
rejection test would have to be deleted.

### 6. The conversion is verified against two references

`sfnt` cannot read every glyph, so it is the reference where it can and the suite asserts how many
glyphs that is, rather than silently comparing less. Three layers:

- **Reader against `sfnt`**: for every glyph `sfnt` can load in the three corpus fonts, our cubic
  segments equal its, in font units.
- **The flex family against hand-computed points**: a charstring built in the test exercises `flex`,
  `hflex`, `flex1` and `hflex1` with coordinates worked out by hand, since no decoder on hand covers
  them.
- **Converted font against the reader**: every stored glyph outline is within one font unit of the
  reader's contours sampled from the source cubics, and `GlyphIndex`, `Advance`, `Kerning`, the
  subset and the determinism of a second conversion are checked separately.

*Why*: the tolerance is a promise, not an implementation detail, and the oracle alone cannot keep it.

### 7. Diagnostics

| Situation | Code |
|---|---|
| A WOFF/WOFF2 file, a CFF2 font, a variable font | the existing unsupported-format diagnostic, naming the format |
| The converted size or a glyph exceeds the cap | `resource/limit` with the font path and the glyph |
| A charstring the reader refuses (`seac`, an operator outside Type 2) | the existing font error, naming the glyph |
| The CFF data is corrupt | the existing font parse error, unmodified |

No new code is introduced: the cases either reuse an existing diagnostic or reuse `resource/limit`.

## Risks / Trade-offs

- **A reader the project now owns** → checked against `sfnt` for every glyph it can load, against
  hand-computed points for the flex family, and with a fuzz seed; the alternative was a vendored
  fork of a module.
- **Outline fidelity** → the fixed quarter-unit fit and the one-unit storage rounding are verified
  point by point; the three corpus fonts are rendered and compared as part of the golden layer.
- **Determinism across architectures** → integer-only subdivision, a fixed `random` and integer
  table assembly; the task requires a repeated-conversion byte comparison, and the integer rule
  makes cross-platform equality structural.
- **Size growth** → the converter's four-times bound plus the existing subsetter; the test records
  the size of a subset of a CFF font.
- **`seac` and CFF2** → refused with a diagnostic naming the cause instead of half-converted.
- **Licensing** → the engine already embeds TrueType subsets and does not consult `fsType`; the same
  policy applies to converted fonts. Recorded as a follow-up, not changed here.
- **No new runtime dependency** → `golang.org/x/image` stays test-only, as it already is for the
  oracle, and the binary does not grow by a decoder.

## Migration Plan

Additive: a document that named a CFF font and failed validation now converts. TrueType output is untouched. Rolling back restores the rejection; no data or format migration is involved.
