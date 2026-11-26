# Fonts: decision and resolution order

## Decision

The engine **does not ship fonts in the binary**. Fonts are part of the document or of the run configuration.

## Why not `go:embed`

- **Licenses.** PT Serif, PT Sans and Liberation are distributed freely, but their redistribution terms differ, and Times New Roman and Arial cannot be redistributed at all. Embedding a font in the binary is distributing it.
- **Size.** One family in four faces is 1.5–3 MB. A full set for documents (serif, sans-serif, mono) easily adds 8–10 MB to the binary.
- **Reproducibility.** Byte-identical PDFs depend on the exact font bytes — and on every other resolved input. The complete input is the engine build, the HTML bytes, the options and the bytes of every linked stylesheet, image and selected font under the same resolution choices. A file edited in place at the same path is a changed input. The engine never selects an installed font on its own: the bytes come from a declared `@font-face` or from an explicit `Options.FallbackFont`/`Options.FallbackFonts`, so the same template and font file reproduce the same output on every machine. Keep the font in the same artifact as the documents, declare it with `@font-face` or pass it as a fallback, and keep the stylesheets and images immutable next to it. `Options.Date` defaults to a fixed constant and the document identifier is a hash of the content, so the clock does not enter the output.

For an integrator this means: the service that embeds the engine decides for itself how to deliver the font — through `go:embed` in its own binary, through a file in the container or through a volume with the fonts.

## Family resolution order

1. `@font-face` in the document — an exact family match, then the closest by weight and slant.
2. `Options.FallbackFont` — a single path to a TrueType or OpenType/CFF file, used for any family that is not declared. It is tried before the list and keeps its single-face meaning.
3. `Options.FallbackFonts` — an ordered list of explicit paths. The first file that loads becomes the family, together with the conventional bold and italic files next to its regular face (`Family-Bold.ttf`, `Family-Italic.ttf`, `DejaVuSans-Bold.ttf`). A file that does not exist or cannot be parsed is skipped and the next entry is tried. One that crosses a documented bound — `MaxFontBytes`, `MaxTotalFontBytes` or the parser's own limit — ends the conversion with `resource/limit` instead of quietly moving on to another family, and a cancelled context returns its cause.
4. The `font/unavailable` error — if nothing is found.

The first three steps use caller-selected bytes, so they keep the output
deterministic. The engine reads **no installed host font on its own**: a
document that declares no usable family and passes no fallback fails with
`font/unavailable` instead of rendering in a font that differs between machines.
The diagnostic names the requested family and both options that supply one.

The pseudo-families `serif`, `sans-serif` and `monospace` without a declared
`@font-face` resolve through step 2 or 3.

### Opting into the host's fonts

`pdfgen.SystemFallbackFonts()` returns the conventional platform paths — Arial
and Times New Roman under the macOS supplemental directory, DejaVu Sans and
Liberation Serif under the Linux font directories — as a fresh copy. Pass the
list explicitly to accept host-dependent output:

```go
res, err := pdfgen.Convert(ctx, src, pdfgen.Options{
	FallbackFonts: pdfgen.SystemFallbackFonts(),
})
```

Paths that do not exist are skipped, so one list works on both platforms. The
returned slice belongs to the caller: modifying or truncating it does not change
another call or a concurrent conversion. The reference command passes this list
so `pdfgen invoice.html` keeps working on a developer machine; that is an example
opt-in, not the library default.

## Embedded assets and `Options.Source`

`pdfgen.Options.Source` accepts any `fs.FS` — `embed.FS`, `os.DirFS`,
`fstest.MapFS` or a subtree made with `fs.Sub` — and replaces the host filesystem
for the whole conversion. Every image, stylesheet and `@font-face` source is read
through it; `BaseDir` and `FontDir` name directories inside it and default to its
root, and `FallbackFont`/`FallbackFonts` are paths inside it, read from the
source root. The two authorities are never mixed: with `Source` set, no host
file and no installed host font is consulted, so ship the font in the source and
declare it with `@font-face` or set `FallbackFont`/`FallbackFonts`. A source
path is slash-separated, relative and must stay inside the source; an absolute
or escaping reference fails with the asset's load diagnostic. With a nil
`Source` (the default) fonts resolve from the host filesystem exactly as
described above, and steps 2 and 3 of the resolution order apply.

## File requirements

- **TrueType** (`.ttf`) and **OpenType with CFF outlines** (`.otf`) are accepted,
  from a local file or from a file in `Options.Source`. WOFF and WOFF2 are
  rejected with a diagnostic naming the format, because they are compressed
  containers the engine does not decode; CFF2, variable fonts and font
  collections are rejected for the same reason.
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
`GPOS`, `GDEF` when the source has one, `hmtx`, `head`, `hhea`, `name`, `OS/2`
and `post` — are kept, and `glyf`, `loca` and `maxp` are synthesized. Shaping,
subsetting, `CIDFontType2` / `FontFile2` embedding and the copyable text layer
then take the same path as a TrueType font, so the PDF object structure does not
change and an existing template converts to the same bytes as before.

`GDEF` travels with the font because the glyph ids do: its mark classes and mark
filtering sets stay valid, so a `GPOS` lookup with `IgnoreMarks` or
`UseMarkFilteringSet` skips the same marks as it does in the source.

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

A composite component id at or beyond `numGlyphs` is an error naming the
composite glyph. The outline reader and the subsetter both refuse it, so a
conversion never embeds a dangling component reference. The check is lazy: an
unused broken glyph does not reject the font, but painting or subsetting the
glyph that references it fails.

Equal `loca` offsets are a legal empty glyph (`space` and some `.notdef` glyphs
have no data) and render as an empty outline. A non-zero range that ends inside
its contour or component data is an error naming the glyph instead of an empty
or partial outline, so a malformed glyph cannot silently disappear from the
page or from the subset.

A CFF charstring delta whose magnitude does not fit an `int16` is rejected with
a diagnostic instead of being truncated: the stored outline must stay consistent
with the header bounding box. A CFF font whose Top DICT omits the `Private`
operator is legal and loads with no local subroutines.

## Parsing bounds

A font file is untrusted input even when it comes from the document. Parsing
charges the conversion's shared work budget for every GPOS lookup, subtable and
coverage entry it examines, and stops with `resource/limit` when the budget is
exhausted. Independent caps bound the counts a font may declare: 4,096 lookups
per font, 4,096 subtables per lookup, 200,000 expanded coverage glyph ids per
font and 1,000,000 scanned coverage entries. Real fonts are far below these
numbers — DejaVu Sans declares two lookups.

Only the `kern`, `mark` and `mkmk` features referenced by the ordinary scripts
apply. The ordinary scripts are `DFLT`, `latn`, `cyrl`, `grek` and `hebr`; a
usable `ScriptList` that names none of them enables no lookup, so a pair
declared only under another script (Arabic, Indic) cannot reach Latin or
Cyrillic text. A missing or out-of-bounds `ScriptList` keeps the fallback the
engine has always had: every lookup stays selectable, so a crafted or
single-script table without a list still applies.

Crossing the coverage cap is `resource/limit`: the parser aborts instead of
keeping the lookups it managed to read, so a font cannot silently lose kerning
by declaring more coverage than the cap. Truncated or inconsistent optional
data is not a cap crossing; it keeps the "no lookups, font loads" behaviour.

Offsets, lengths and counts are evaluated in 64-bit arithmetic before any slice,
so a value that does not fit an `int` on a 32-bit build is rejected instead of
wrapping; a `cmap` format 12 group that would map past glyph 65535 is refused.

A malformed optional table degrades rather than rejecting the font. A `GPOS`
`FeatureList` offset outside the table disables every lookup, while a
`ScriptList` offset outside the table keeps the all-lookups fallback; either way
the font loads, no byte outside the table is read, and parsing never panics.

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
