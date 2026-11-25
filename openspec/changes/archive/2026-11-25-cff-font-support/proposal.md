# Proposal

## Why

The profile accepts only TrueType. A large share of the fonts business documents name — PT Serif, PT Sans, many modern families — ship as OpenType with CFF outlines, and the engine rejects them at validation, forcing the author to convert the font before the template can work. The reason recorded in the profile is that CFF outlines need a separate interpreter, and the engine has none: its writer embeds TrueType glyphs, and `golang.org/x/image/font/sfnt`, the decoder the tests already use as an oracle, implements the Type 2 charstring operators only up to `hflex`/`hflex1`, so it cannot read `flex` or `flex1` — operators the corpus fonts use in ordinary characters such as `8` in Barlow Condensed Bold.

## What Changes

- `@font-face` and `Options.FallbackFont` SHALL accept OpenType fonts with CFF outlines (`.otf`) in addition to TrueType. WOFF, WOFF2 and remote font URLs SHALL stay rejected.
- The engine SHALL read CFF outlines itself: the `CFF ` table and a Type 2 charstring interpreter in `internal/font`, with `golang.org/x/image/font/sfnt` kept as a test-only oracle.
- A CFF font SHALL be converted to a TrueType font in memory before layout: the same glyph IDs; the original `cmap`, `GPOS`, `name`, `OS/2`, `post`, `hhea` and `hmtx` tables; synthesized `glyf`, `loca` and `maxp`; and a patched `head`. Cubic outlines SHALL become quadratic ones within a stated tolerance, deterministically.
- The converted font SHALL then follow the existing path: shaping, subsetting, `CIDFontType2`/`FontFile2` embedding and `ToUnicode`. The PDF object structure SHALL NOT change.
- The conversion SHALL be bounded: a font whose converted form exceeds the documented limit MUST fail with `resource/limit` instead of allocating without bound, and the same input bytes MUST produce the same converted font.
- `docs/fonts.md` and the profile documentation SHALL state that OpenType/CFF is accepted through conversion and what the approximation means for outline fidelity.

Not included: native `CIDFontType0`/`FontFile3` embedding, CFF subsetting, CFF2 and variable fonts, WOFF/WOFF2 decoding, `seac` accent composition, and an `fsType` embedding-permission check (recorded as a follow-up).

## Capabilities

### Modified Capabilities

- `text-shaping`: the TrueType-only requirement is replaced by one that accepts local TrueType and OpenType/CFF fonts and keeps the rejection rules for WOFF, WOFF2 and remote sources.

## Impact

- **Dependencies**: none at runtime. `golang.org/x/image` stays the test-only oracle it already is, so the published package's build closure does not change; the binary grows by the engine's own reader, not by a decoder module.
- **Code**: `internal/font` (the CFF table reader, the Type 2 charstring interpreter, the converter, outline encoding, table assembly), `internal/engine` (font loading and the limit), and the diagnostic that reports it.
- **Tests**: the three `barlowcondensed-*.otf` files already in the corpus; text extraction, subsetting, determinism, the limit and WOFF rejection.
- **Documentation**: `docs/fonts.md`, `docs/profile.md`, the README's font section, CHANGELOG.
- **Compatibility**: additive; existing TrueType documents keep their output byte for byte.
