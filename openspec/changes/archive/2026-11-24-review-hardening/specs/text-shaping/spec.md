# Spec Delta

## ADDED Requirements

### Requirement: Malformed optional font tables do not crash parsing

Parsing a font with a truncated or inconsistent optional table SHALL NOT panic. Every field read MUST be preceded by a bounds check for that table format. A corrupt optional table MAY be ignored, in which case the font loads without the feature it provides; a corrupt required table MUST produce an error.

#### Scenario: Truncated class definition
- **WHEN** a class definition in a GPOS table ends before its range header
- **THEN** parsing does not panic
- **AND** the font either loads without kerning or returns an error

#### Scenario: Truncated kerning table
- **WHEN** the GPOS table is truncated in the middle of a pair set
- **THEN** parsing does not panic
- **AND** the font loads with no kerning pairs

#### Scenario: Required table missing
- **WHEN** a required table such as `glyf` is absent
- **THEN** parsing returns an error naming the missing table
