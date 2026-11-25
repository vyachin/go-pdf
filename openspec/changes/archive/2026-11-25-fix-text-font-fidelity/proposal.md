# Proposal

## Why

Text and font handling loses fidelity on inputs the profile accepts. A combining mark
that has no precomposed form is drawn after the base glyph at the pen, so `сло́во`
renders the accent on the next letter; GPOS MarkBase/MarkMark lookups (types 4–6) are
ignored. Form feed, U+2028 and U+2029 are not line breaks, so a form feed inside a
word fails with `text/glyph`, and a line separator joins two lines. A CFF delta above
32767 wraps to a negative int16 and silently corrupts the glyph, a composite glyph
ignores `SCALED_COMPONENT_OFFSET`, and a CFF font without a `Private` DICT is refused
although the project's own oracle loads it.

## What Changes

- A combining mark without a precomposed form SHALL be positioned over its base
  through GPOS mark attachment, or rejected with a diagnostic; it MUST NOT be drawn
  at the pen.
- `\f`, U+2028 and U+2029 SHALL be line breaks in text layout.
- A CFF outline delta outside the int16 range SHALL be rejected, not wrapped.
- `SCALED_COMPONENT_OFFSET` SHALL transform the component offset.
- A CFF font without a `Private` DICT SHALL load with no local subroutines.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: mark positioning, Unicode separators, CFF range checks and
  composite offset flags follow the font and Unicode semantics.

## Impact

- **Code**: `internal/text/shape.go`, `normalize.go`, `internal/font/gpos.go`,
  `glyph.go`, `cff.go`, `cff_convert.go`.
- **Output**: stressed Cyrillic renders correctly; text with separators keeps its
  lines; corrupt CFF values fail instead of producing a wrong glyph.
- **Tests**: mark-position, separator, delta-overflow, scaled-offset and
  missing-Private fixtures.
