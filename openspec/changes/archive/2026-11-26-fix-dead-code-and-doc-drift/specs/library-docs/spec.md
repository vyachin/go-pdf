# Spec Delta

## ADDED Requirements

### Requirement: The profile document is a checked reference (RV5-47)

The profile document SHALL remain the complete list it claims to be: its element,
attribute and inherited-property tables MUST match the code that validates them,
the `style` element MUST appear with its `media` attribute, and the `media`
wording and the `line-height` row MUST match the parser. A test SHALL compare the
tables with the code so a change to one side fails.

#### Scenario: The attribute table is complete

- **WHEN** the documented attribute table is compared with the allow-list
- **THEN** `style` appears with `media`
- **AND** every other documented row names an attribute the code accepts

#### Scenario: Media semantics are stated as implemented

- **WHEN** a reader applies the documented `media` rule to `media="only print"`
- **THEN** the document says the sheet is skipped, matching the whole-item match
- **AND** `media="print"` and `media="all"` remain described as applying

#### Scenario: The line-height row lists percentage

- **WHEN** the documented `line-height` value list is compared with the parser
- **THEN** it lists a number, a length, a percentage and `normal`

#### Scenario: The documentation drifts

- **WHEN** an element, attribute or inherited flag changes in code but not in
  the document (or the reverse)
- **THEN** the documentation test fails and names the drifted row
