# Spec Delta

## ADDED Requirements

### Requirement: The public API mirrors the internal one (RV5-52)

A test SHALL compare `pdfgen.Options`, `pdfgen.Limits` and `pdfgen.Diagnostic`
with their internal counterparts field by field — name, type and count — and
verify that every field reaches the internal value and back;
`DefaultLimits` SHALL equal the engine defaults. The godoc of a mirrored field
MUST state the semantics of the field it mirrors, including `MaxTableCells` as a
grid area and `RenderPNG` as a debug preview.

#### Scenario: A field is added on one side only

- **WHEN** a field is added to `engine.Options` or `engine.Limits` and not to
  the public mirror, or the other way round
- **THEN** the mirror test fails and names the field and the type

#### Scenario: A field is not mapped

- **WHEN** a public field is missing from `toEngine`, `fromEngineLimits` or
  `fromDiags`
- **THEN** the round-trip test fails for that field
- **AND** the failure names the public field and the direction

#### Scenario: Defaults are compared reflectively

- **WHEN** the default limits are compared
- **THEN** every field of `pdfgen.DefaultLimits()` equals the engine default
- **AND** a new limit cannot be added without entering the comparison

#### Scenario: Mirrored godoc is corrected

- **WHEN** the documentation of `MaxTableCells` and `RenderPNG` is inspected
- **THEN** `MaxTableCells` names the grid area (columns times rows)
- **AND** `RenderPNG` describes a debug preview, not a CLI flag
- **AND** the documentation test fails if the CLI claim returns
