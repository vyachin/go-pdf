# Spec Delta

## ADDED Requirements

### Requirement: A ScriptList with no ordinary script enables no lookups (RV5-28)

Lookup selection SHALL distinguish a missing or unusable `ScriptList` from a
usable list that names none of the ordinary scripts. The former MAY keep the
existing all-lookups fallback; the latter MUST enable no lookup, so a pair defined
only under another script cannot apply to Latin or Cyrillic text.

#### Scenario: An only-arab ScriptList

- **WHEN** a font declares one GPOS script `arab` with a `kern` pair of -300 and
  the document contains Latin `AV`
- **THEN** no lookup is enabled and the pair is not applied
- **AND** the font loads without a diagnostic

#### Scenario: An ordinary script still applies

- **WHEN** the same font also declares `DFLT` or `latn` with that pair
- **THEN** the pair is applied to Latin text

#### Scenario: No usable ScriptList

- **WHEN** the `ScriptList` offset is missing or out of bounds
- **THEN** the existing fallback keeps every lookup selectable
- **AND** a crafted single-script table without a list still applies its pairs

### Requirement: Exceeding the coverage cap returns resource/limit (RV5-29)

When lookup parsing would exceed the documented glyph-coverage cap, the parser
MUST return a `resource/limit` error. It MUST NOT return a partial set of enabled
lookups, so a font cannot silently lose kerning by declaring more coverage than
the cap.

#### Scenario: The fourth oversized lookup is refused

- **WHEN** a font declares four `kern` lookups and each covers 60,000 glyphs
- **THEN** the font load fails with `resource/limit`
- **AND** the engine does not apply a subset of three lookups

#### Scenario: A font within the cap loads

- **WHEN** the same lookups fit within the coverage cap
- **THEN** the font loads with all of them enabled
- **AND** kerning matches a font with equivalent tables

#### Scenario: Malformed is not the cap

- **WHEN** a lookup is truncated rather than over the cap
- **THEN** the font loads without that lookup, as the optional-table rule allows
- **AND** the outcome is not reported as `resource/limit`

### Requirement: A composite component beyond numGlyphs is rejected (RV5-30)

A composite glyph component whose glyph id is at or beyond `numGlyphs` MUST be an
error naming the glyph. The parser and the subsetter MUST NOT drop the component
and continue, and the embedded subset MUST NOT contain a dangling component
reference.

#### Scenario: A dangling component

- **WHEN** a used composite glyph references component id `numGlyphs`
- **THEN** the conversion reports an error naming the glyph
- **AND** no subset with the dangling reference is written

#### Scenario: A valid composite is unchanged

- **WHEN** every component id is inside `numGlyphs`
- **THEN** the outline and the embedded subset keep every component
- **AND** the output equals the current output for the same font

### Requirement: A truncated glyph is an error, not an empty outline (RV5-31)

The glyph reader SHALL distinguish an empty glyph (equal `loca` offsets, legal)
from a truncated glyph (malformed). A malformed simple or composite glyph MUST
produce an error naming the glyph; it MUST NOT return an empty or partial outline.

#### Scenario: A truncated simple glyph

- **WHEN** a used glyph's `loca` range ends inside its contour data
- **THEN** the conversion reports an error naming the glyph
- **AND** the glyph is not silently replaced by an empty outline

#### Scenario: A legal empty glyph

- **WHEN** a glyph's `loca` offsets are equal, as for `space`
- **THEN** it renders as an empty glyph with no error

#### Scenario: A truncated composite glyph

- **WHEN** a used composite's component data ends before its last component
- **THEN** the conversion reports an error
- **AND** the subset never embeds a partial component list as if complete

### Requirement: CFF conversion copies GDEF when present (RV5-32)

Converting a CFF font to TrueType SHALL copy the `GDEF` table when the source has
one, because glyph numbering is preserved. Mark classes and mark filtering sets
must survive the conversion so `IgnoreMarks` and `UseMarkFilteringSet` behave as
they do for the original font.

#### Scenario: A CFF font with GDEF

- **WHEN** an OpenType/CFF font with a 254-byte GDEF and `IgnoreMarks` pair lookups
  is loaded
- **THEN** the converted font contains `GDEF`
- **AND** the pair lookups skip mark glyphs as the source declares

#### Scenario: A CFF font without GDEF

- **WHEN** the source font has no `GDEF`
- **THEN** the converted font has none
- **AND** the table set otherwise matches the current conversion
