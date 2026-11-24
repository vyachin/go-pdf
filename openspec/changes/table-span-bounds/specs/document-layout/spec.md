# Spec Delta

## ADDED Requirements

### Requirement: Table spans are bounded

`colspan` and `rowspan` SHALL be validated before layout uses them: a value MUST be an integer in `1..100`, and any other value MUST produce an error diagnostic naming the attribute and the cell. Layout MUST clamp every span it reads, so no span can reach integer conversion, division or grid allocation unchecked.

#### Scenario: Overflowing rowspan
- **WHEN** a cell declares `rowspan="4294967296"`
- **THEN** conversion fails with an `html/invalid-span` diagnostic
- **AND** no panic occurs

#### Scenario: Huge colspan
- **WHEN** a cell declares a `colspan` above the limit
- **THEN** conversion fails with a diagnostic naming the attribute

#### Scenario: Ordinary spans
- **WHEN** a table uses `rowspan="2"` and `colspan="3"`
- **THEN** layout places the cells as before
