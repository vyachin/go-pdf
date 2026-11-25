# Spec Delta

## ADDED Requirements

### Requirement: Combining marks are positioned over their base

When normalisation leaves a combining mark as a separate code point, the engine SHALL
position it with the font's GPOS mark attachment (`MarkBase`, `MarkLig`, `MarkMark`)
when those lookups exist, so it is drawn over the base glyph rather than at the pen.
When the font cannot attach it, the engine MUST report a diagnostic instead of
silently drawing it at the wrong place.

#### Scenario: A stressed Russian word

- **WHEN** the text contains "сло́во" with U+0301 after "о"
- **THEN** the acute is drawn over "о"
- **AND** extraction still returns the combining sequence

#### Scenario: A font without mark attachment

- **WHEN** the font has no GPOS mark lookups for a separate mark
- **THEN** the document fails with a diagnostic naming the mark

### Requirement: Unicode separators break lines

Form feed (U+000C), line separator (U+2028) and paragraph separator (U+2029) SHALL
start a new line. A form feed MUST NOT reach glyph lookup, and no separator may join
two lines in the rendered output.

#### Scenario: A form feed in preformatted text

- **WHEN** `white-space: pre` text contains `A\fB`
- **THEN** two lines are rendered and no `text/glyph` error is reported

#### Scenario: A line separator

- **WHEN** text contains `A` U+2028 `B`
- **THEN** two lines are rendered

### Requirement: CFF values outside the TrueType range are rejected

A CFF charstring delta whose magnitude exceeds 32767 SHALL be rejected with a
diagnostic, never truncated to int16. The glyph's stored outline MUST stay consistent
with its header bounding box.

#### Scenario: An oversized delta

- **WHEN** a charstring draws a 32767.99998-unit line
- **THEN** loading the font fails with a diagnostic
- **AND** the glyph is not silently drawn with a 65,536-unit jump

### Requirement: Composite components apply scaled offsets

A composite glyph component whose flags include `SCALED_COMPONENT_OFFSET` (0x0800)
SHALL have its offset transformed by the component's scale before it is added.

#### Scenario: A scaled offset

- **WHEN** a component at offset (100, 0) with scale 0.5 sets the flag
- **THEN** the first point is the child's point plus 50, not plus 100

### Requirement: A CFF font without a Private DICT loads

A CFF font whose top DICT omits the `Private` operator SHALL load as if it declared an
empty private dictionary, with no local subroutines. It MUST NOT be rejected.

#### Scenario: A minimal CFF font

- **WHEN** an otherwise valid CFF font has no `Private` operator
- **THEN** the font loads and glyph 0 is drawn
