# Spec Delta

## ADDED Requirements

### Requirement: The extractor validates the structure it reads (RV4-30)

The PDF debug extractor SHALL validate `startxref`, the cross-reference entries and
each stream's `/Length`, or delegate that check to an independent validator, and it
MUST NOT return the same result for a structurally corrupt file as for a valid one.
The extractor MUST NOT be the only check on the writer: a glyph-level or render-level
check MUST cover what text extraction cannot see.

#### Scenario: A stream length off by seven

- **WHEN** every `/Length` in a produced PDF is increased by seven
- **THEN** the extractor reports the file as structurally invalid
- **AND** the test suite fails even though the text would still extract

#### Scenario: A broken CID-to-GID map

- **WHEN** a PDF's `/CIDToGIDMap` is zeroed without changing its length
- **THEN** a render-level check catches the missing glyphs
- **AND** the text extractor alone is not treated as proof of correctness

### Requirement: An independent validator checks writer output (IMP-08)

The writer's output SHALL be checked by a validator that does not share the writer's
parsing assumptions, and the debug extractor MUST NOT be the only check. The
structural checks MUST cover the cross-reference table, stream lengths and glyph
mapping.

#### Scenario: A corrupted structure (IMP-08)

- **WHEN** a produced PDF has a wrong `/Length` or a shifted xref
- **THEN** the independent check fails
- **AND** a text-only comparison does not certify the file

