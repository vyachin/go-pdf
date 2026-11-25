# Spec Delta

## ADDED Requirements

### Requirement: GPOS pair expansion is capped and charged (RV4-01)

A `PairPos` subtable SHALL be bounded by a declared-pair cap and MUST charge the
work guard before it expands the pairs into a map. A font whose pair tables exceed
the cap MUST fail with `resource/limit` before the map is built, instead of
allocating a map entry per pair.

#### Scenario: A small font with a large pair table

- **WHEN** a 264 KB font declares a coverage of 1024 glyphs and a PairSet of 65535
  pairs per covered glyph
- **THEN** the conversion fails with `resource/limit` before allocating memory
  proportional to the expanded pair count
- **AND** the same font with a small pair table still converts

#### Scenario: The existing crafted-GPOS fixture stays green

- **WHEN** the corpus runs `p1-crafted-gpos.html` under `MaxOperations: 250`
- **THEN** it still reports `resource/limit`
- **AND** the new pair-table fixture is registered beside it

### Requirement: CFF outline conversion honours the guard and the context (RV4-05)

Converting CFF outlines to TrueType SHALL charge the work guard per glyph and per
instruction, MUST check the guard between glyphs, and MUST observe context
cancellation during the conversion. A font's whole conversion MUST be bounded by
the same budget as its parsing.

#### Scenario: A deep subroutine chain is bounded

- **WHEN** a 459 KB OTTO font has 65535 charstrings whose subroutines expand to
  hundreds of thousands of instructions
- **THEN** the conversion stops with `resource/limit` after work proportional to the
  budget
- **AND** cancelling the context during the conversion returns the context error
  promptly, not after the whole font is converted

### Requirement: A non-breaking space stays U+00A0 in the text layer (RV4-31)

Normalization SHALL keep a non-breaking space as U+00A0 in the glyph run and in the
`ToUnicode` map while still treating it as unbreakable, so that copied text carries
the character the author wrote.

#### Scenario: Copying a non-breaking space

- **WHEN** a document contains `a&nbsp;b`
- **THEN** the extracted text contains U+00A0 between the two letters
- **AND** the line is not broken at that space

### Requirement: A composite glyph without XY arguments is not placed as an offset (RV4-40)

When a composite component does not set `ARGS_ARE_XY_VALUES` (0x0002), the parser
MUST NOT treat its two arguments as an offset. It SHALL reject the glyph with a
diagnostic or implement point matching, and the result MUST be an error rather than
a silently wrong outline.

#### Scenario: A point-matching composite

- **WHEN** a font contains a composite glyph whose second component omits
  `ARGS_ARE_XY_VALUES` and supplies point indices
- **THEN** the glyph is rejected with a diagnostic or placed by point matching
- **AND** it is never placed at the point-index numbers as coordinates

### Requirement: A too-deep composite glyph is an error, not an empty outline (RV4-41)

A composite glyph beyond the depth limit SHALL be reported as unsupported. The
subsetter MUST keep every component of a glyph the document uses; it MUST NOT drop
the components because a depth cap returned an empty outline.

#### Scenario: A long composite chain

- **WHEN** a document uses a glyph that is a twelve-deep composite chain
- **THEN** the glyph either renders with its outline or fails with a diagnostic
- **AND** the embedded subset contains the components of every used glyph

### Requirement: Only the font's default kern, mark and mkmk lookups apply (RV4-42)

GPOS application SHALL select lookups through `ScriptList`, `FeatureList` and
`lookupFlag`, enabling the default-script `kern`, `mark` and `mkmk` features and
honouring `IgnoreMarks` and mark filtering sets. A lookup that belongs to another
script or feature MUST NOT be applied to ordinary text.

#### Scenario: A script-specific lookup is ignored

- **WHEN** a font declares a `PairPos` lookup only under a non-default script
- **THEN** the engine does not apply it to Latin or Cyrillic text
- **AND** `kern` lookups of the default script still apply

### Requirement: The font face cache distinguishes family lists (RV4-43)

The face cache MUST key on a family list unambiguously, so `"a b"` and `a, b` at
the same weight, slant and size cannot share an entry. A cache hit MUST return the
face the requested list resolves to.

#### Scenario: A family whose name contains a space

- **WHEN** a document uses `font-family: "a b"` and another uses `font-family: a, b`
  in the same conversion
- **THEN** each resolves to its own face
- **AND** the second request does not receive the first family's face

### Requirement: Every font load path is charged (RV4-44)

System-fallback parsing SHALL charge the work guard exactly like an `@font-face`
file, including the CFF conversion it may trigger. The budget MUST account for
every font the conversion parses, whatever the path that named it.

#### Scenario: A fallback font is parsed

- **WHEN** a document without `@font-face` resolves a family through the system
  fallback
- **THEN** the parse is charged to the guard
- **AND** an exhausted budget stops the fallback load with `resource/limit`
