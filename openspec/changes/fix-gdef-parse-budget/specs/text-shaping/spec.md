# Spec Delta

## ADDED Requirements

### Requirement: GDEF class expansion is charged before it expands (RV5-15)

Parsing a GDEF table SHALL charge the shared work guard for every class range it
reads and for every glyph entry it expands, before the entry is written to the
mark map. A font whose GDEF expansion would exceed the budget MUST fail with
`resource/limit` instead of expanding the ranges.

#### Scenario: Overlapping class ranges stop at the budget

- **WHEN** a 12 KB font declares 2,000 overlapping class-3 ranges and the
  conversion runs with a budget smaller than their expansion
- **THEN** the font load stops with `resource/limit`
- **AND** the guard records work proportional to the ranges examined, not zero

#### Scenario: A normal font is unaffected

- **WHEN** a regular text font with a small GDEF is loaded under the default
  limits
- **THEN** it loads with its mark data and no diagnostic
- **AND** the charge is far below the default budget

#### Scenario: Entries, not only ranges, are charged

- **WHEN** one range covers 60,000 glyphs and another covers one
- **THEN** the charge for the wide range is larger than the charge for the narrow
  one
- **AND** a budget that admits the narrow range refuses the wide one

### Requirement: GDEF parsing observes cancellation (RV5-15)

The parser SHALL check the guard between class ranges and mark-set coverage
entries, so a cancelled context stops the parse instead of returning a font after
the whole table. The returned error MUST be the context error, not a
`resource/limit` diagnostic.

#### Scenario: Cancellation in the middle of a parse

- **WHEN** the context is cancelled 20 ms after a GDEF-heavy font starts parsing
- **THEN** the load returns the context error promptly
- **AND** it does not return a successfully parsed font

#### Scenario: A pre-cancelled context

- **WHEN** the context is already cancelled before the font is opened
- **THEN** the load returns the context error without parsing the GDEF table

### Requirement: GDEF expansion is capped (RV5-15)

A font's total expanded mark entries SHALL be bounded by a documented cap. A font
whose ranges expand beyond the cap MUST fail with `resource/limit`; the parser
MUST NOT keep a partial mark set that silently changes which glyphs are treated
as marks.

#### Scenario: A font at the format maximum

- **WHEN** a GDEF table declares 65,535 overlapping class-3 ranges
- **THEN** the load fails with `resource/limit` after bounded work
- **AND** no mark map larger than the cap is allocated

#### Scenario: A cap crossing is not a partial parse

- **WHEN** the expansion crosses the cap in the middle of a table
- **THEN** the whole font is refused
- **AND** the converter never applies a mark set built from only the first ranges

### Requirement: A GDEF coverage limit is propagated (RV5-15)

A `resource/limit` returned while reading a GDEF mark-set coverage SHALL be
propagated, not swallowed. The conversion MUST stop with `resource/limit`. A
malformed optional table that is not a resource refusal MUST keep the existing
degrade behaviour: the font loads without GDEF mark data.

#### Scenario: A mark-set coverage over the cap

- **WHEN** a GDEF mark set covers more glyphs than the font coverage cap allows
- **THEN** the font load fails with `resource/limit`
- **AND** the parser does not continue with that set dropped

#### Scenario: Malformed GDEF still degrades

- **WHEN** the GDEF table is truncated or inconsistent but does not cross a
  resource bound
- **THEN** the font loads without mark data
- **AND** no diagnostic is required for the optional table
