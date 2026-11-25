# Spec Delta

## ADDED Requirements

### Requirement: Font parsing work is bounded

Font parsing SHALL charge the shared work guard for every lookup, subtable and
coverage range it examines, and SHALL stop with a `resource/limit` diagnostic when
the configured budget is exhausted. The number of entries a font declares MUST NOT
let a small file bypass the budget, and parsing MUST observe context cancellation.

#### Scenario: The amplifier font stops at the budget

- **WHEN** a font whose GPOS table repeats one PairPos across 40,000 subtables is
  loaded with a work budget that the parse would exceed
- **THEN** the call returns `resource/limit` instead of parsing every subtable

#### Scenario: A normal font is unaffected

- **WHEN** a regular text font is parsed under the default limits
- **THEN** the font loads with its kerning pairs and no diagnostics

#### Scenario: Cancellation during parsing

- **WHEN** the context is cancelled while a large font is parsed
- **THEN** the call returns the context error

### Requirement: Font-declared sizes never overflow

Every offset, length and count read from a font SHALL be evaluated in 64-bit
arithmetic before it is converted or used to slice, on every supported architecture.
A value outside its table or the glyph-id range MUST be rejected, and a `cmap`
format 12 group whose `startGlyphID + count` exceeds 0xFFFF MUST be rejected.

#### Scenario: A high offset on a 32-bit build

- **WHEN** a font declares a `cmap` offset of 0x80000000 on a 32-bit build
- **THEN** the font is rejected with a parse error and the process does not panic

#### Scenario: Format 12 glyph id overflow

- **WHEN** a format 12 group starts at `startGlyphID: 0xFFFFFFF0` and spans several
  characters
- **THEN** parsing rejects the group instead of mapping the characters to low glyphs
