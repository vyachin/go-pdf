# Spec Delta

## ADDED Requirements

### Requirement: Default limits are readable by callers

The `pdfgen` package SHALL export the limits a zero `Limits` value resolves to, so a
caller that reads its own input can reject an oversized file before loading it. The
exported values MUST equal the values the conversion uses and MUST track changes to
the defaults.

#### Scenario: Defaults match the conversion

- **WHEN** a caller reads `pdfgen.DefaultLimits()`
- **THEN** every field equals the value the same conversion applies with `Options{}`

#### Scenario: The command uses the export

- **WHEN** the reference command checks an input's size
- **THEN** it uses the exported defaults rather than a hard-coded constant
