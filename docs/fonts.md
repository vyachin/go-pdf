# Fonts: decision and resolution order

## Decision

The engine **does not ship fonts in the binary**. Fonts are part of the document or of the run configuration.

## Why not `go:embed`

- **Licenses.** PT Serif, PT Sans and Liberation are distributed freely, but their redistribution terms differ, and Times New Roman and Arial cannot be redistributed at all. Embedding a font in the binary is distributing it.
- **Size.** One family in four faces is 1.5–3 MB. A full set for documents (serif, sans-serif, mono) easily adds 8–10 MB to the binary.
- **Reproducibility.** Byte-identical PDFs depend on the exact font bytes — and on every other resolved input. The complete input is the engine build, the HTML bytes, the options and the bytes of every linked stylesheet, image and selected font under the same resolution choices. A file edited in place at the same path is a changed input, and system fallback selects different bytes on different machines. Keeping the font in the same artifact as the documents is more reliable than depending on the system font; pin `Options.FallbackFont` or declare `@font-face`, and keep the stylesheets and images immutable next to it. `Options.Date` defaults to a fixed constant and the document identifier is a hash of the content, so the clock does not enter the output.

For an integrator this means: the service that embeds the engine decides for itself how to deliver the font — through `go:embed` in its own binary, through a file in the container or through a volume with the fonts.

## Family resolution order

1. `@font-face` in the document — an exact family match, then the closest by weight and slant.
2. `Options.FallbackFont` — a path to a TrueType or OpenType/CFF file, used for any family that is not declared.
3. System fonts from `DefaultFallbackFonts` — the first available family, together with the conventional bold and italic files next to its regular face (`Arial Bold.ttf`, `DejaVuSans-Bold.ttf`, `LiberationSerif-Bold.ttf`). This is a convenience for development, not for production: the set of system fonts is not controlled.
4. The `font/unavailable` error — if nothing is found.

The pseudo-families `serif`, `sans-serif` and `monospace` without a declared `@font-face` resolve through step 2 or 3.

## File requirements

- **TrueType** (`.ttf`) and **OpenType with CFF outlines** (`.otf`) are accepted, from a local file only. WOFF and WOFF2 are rejected with a diagnostic naming the format, because they are compressed containers the engine does not decode; CFF2, variable fonts and font collections are rejected for the same reason.
- Cyrillic is mandatory: a character without a glyph is the error `text/glyph`, not a silent replacement.
- `@font-face` accepts only a local `url(...)`; `format()` and remote URLs are rejected.

### Embedded subset name and descriptor

The embedded subset is named `XXXXXX+Family` in both `/BaseFont` and
`/FontName`, where `XXXXXX` is six uppercase letters derived deterministically
from the subset bytes. `pdffonts` therefore reports the font as a subset.

`/CapHeight` is the font's `OS/2` `sCapHeight`, scaled to 1/1000 em; when the
table does not declare it, the height of 'H' is measured. `/ItalicAngle` comes
from the `post` table, so an oblique or italic face does not report 0. A font
without either table keeps a safe value: the ascent for the cap height and 0
for the angle.

### OpenType with CFF outlines

An `.otf` whose outlines are PostScript (CFF) is converted to TrueType in memory
before layout. The glyph numbering and the tables that depend on it — `cmap`,
`GPOS`, `hmtx`, `head`, `hhea`, `name`, `OS/2` and `post` — are kept, and `glyf`,
`loca` and `maxp` are synthesized. Shaping, subsetting, `CIDFontType2` /
`FontFile2` embedding and the copyable text layer then take the same path as a
TrueType font, so the PDF object structure does not change and an existing
template converts to the same bytes as before.

The engine reads the `CFF ` table and its Type 2 charstrings itself. It does not
depend on an external decoder at run time: `golang.org/x/image`, the decoder the
tests compare against, stays a test-only requirement.

The conversion is an approximation, and these numbers are the promise:

| Quantity | Value | Meaning |
|---|---|---|
| Fit tolerance | 16 fixed-point units, i.e. one quarter of a font unit (0.00025 em at 1000 units per em) | every quadratic fitted to a source cubic stays within this distance of it |
| Stored outline | one font unit (0.001 em) | `glyf` stores whole units, so a stored point is the fitted point rounded; the stored outline stays within one unit of the source curve |
| Point cap | 8192 points per glyph | a glyph that would need more fails with `resource/limit` naming the glyph |
| Converted size | four times the input, and never above 64 MiB | a `glyf` denser than that fails with `resource/limit` naming the glyph that crossed the bound |

The conversion is integer-only, with no floating point in the result, so the same
font bytes produce the same glyph data and the same PDF on every platform.

Two limitations are deliberate. A glyph that uses the deprecated Type 1 `seac`
accent composition is refused with a diagnostic rather than drawn without its
accent, and a glyph whose charstring uses an operator outside the Type 2 set is
refused the same way; the `flex`, `hflex`, `flex1` and `hflex1` operators, the
curve and line families, subroutines and the arithmetic operators are all
implemented. The `fsType` flag of the `OS/2` table is not consulted, exactly as it
is not consulted for TrueType: the engine does not know the licence the document
author holds, so check the font's licence before shipping a template.

Subsetting a converted font uses the existing TrueType path and keeps the glyph
numbering, so the embedded program carries only the outlines the document uses.

## Marks and separators

Text is normalised to NFC first. A combining mark that has no precomposed form
stays a separate code point, and the shaper positions it with the font's GPOS
mark attachment: `MarkBase`, `MarkLig` and `MarkMark` lookups are parsed, and
the mark's anchor is placed on the base or preceding mark anchor. In a font
without a usable lookup for the reason the conversion stops with a `text/glyph`
diagnostic naming the mark; drawing it at the pen would put the accent on the
next letter. The copyable text layer still extracts the base and the mark as
the original combining sequence.

Form feed (U+000C), line separator (U+2028) and paragraph separator (U+2029)
start a new line in both preformatted and normal text. A form feed never
reaches glyph lookup, and no separator joins two rendered lines.

## Composite and CFF boundaries

A composite glyph component whose flags include `SCALED_COMPONENT_OFFSET`
(0x0800) has its offset transformed by the component's scale before it is added,
so a component at (100,0) with scale 0.5 lands at (50,0).

A CFF charstring delta whose magnitude does not fit an `int16` is rejected with
a diagnostic instead of being truncated: the stored outline must stay consistent
with the header bounding box. A CFF font whose Top DICT omits the `Private`
operator is legal and loads with no local subroutines.

## Parsing bounds

A font file is untrusted input even when it comes from the document. Parsing
charges the conversion's shared work budget for every GPOS lookup, subtable and
coverage entry it examines, and stops with `resource/limit` when the budget is
exhausted. Independent caps bound the counts a font may declare: 4,096 lookups
per font, 4,096 subtables per lookup and 1,000,000 scanned coverage entries.
Real fonts are far below these numbers — DejaVu Sans declares two lookups.

Offsets, lengths and counts are evaluated in 64-bit arithmetic before any slice,
so a value that does not fit an `int` on a 32-bit build is rejected instead of
wrapping; a `cmap` format 12 group that would map past glyph 65535 is refused.

## Recommended set for Russian documents

| Role | Font | License |
|---|---|---|
| Body text | PT Serif | ParaType Free Font License |
| Headings | PT Sans | ParaType Free Font License |
| Office compatibility | Liberation Serif / Sans | SIL OFL |
| Wide coverage | DejaVu Sans | free |

## Verification

```go
// FontDir is the directory relative to which @font-face paths resolve.
opts := pdfgen.Options{FontDir: "fonts"}

// Validate checks that the document references only local TrueType or
// OpenType/CFF fonts.
diags, err := pdfgen.Validate(ctx, src, opts)

// Convert renders with the same resolution rules.
res, err := pdfgen.Convert(ctx, src, opts)
```

The reference command sets `FontDir` to the input file's directory, so a font
placed beside the document is found without any configuration.
