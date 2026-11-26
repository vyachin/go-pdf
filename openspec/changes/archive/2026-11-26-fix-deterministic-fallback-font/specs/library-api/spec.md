# Spec Delta

## ADDED Requirements

### Requirement: The default font is host-independent (RV5-53)

With no matching `@font-face` and no caller-selected font, conversion SHALL fail with
`font/unavailable` and MUST NOT read an installed host font. Host fonts SHALL be selectable
only through explicit `Options` (`FallbackFont` or `FallbackFonts`), and the package's
suggested system list MUST be returned by value so no caller can mutate package state.
Identical inputs MUST produce identical bytes on hosts with different installed fonts, and
the documentation MUST state the resolution order.

#### Scenario: Two hosts with different system fonts

- **WHEN** the same document with no `@font-face` and no fallback converts on a host with
  Arial and on a host with DejaVu Sans
- **THEN** both calls fail with `font/unavailable`
- **AND** neither host's font bytes appear in any output
- **AND** the two diagnostic lists agree

#### Scenario: An explicit fallback still renders

- **WHEN** the caller sets `FallbackFont` or `FallbackFonts` to a font file
- **THEN** the document converts in that face, including its bold and italic siblings
- **AND** two runs on different hosts produce identical bytes

#### Scenario: The reference command opts in explicitly

- **WHEN** `pdfgen invoice.html` runs on a developer machine
- **THEN** the command passes `SystemFallbackFonts()` as an explicit list
- **AND** the library default remains deterministic

#### Scenario: The suggested list cannot be mutated

- **WHEN** a caller modifies the slice returned by `SystemFallbackFonts()` and converts
  again
- **THEN** the package's list is unchanged
- **AND** the second conversion selects the same font bytes as the first

#### Scenario: A declared family keeps priority

- **WHEN** a document declares `@font-face` for the requested family and also sets a
  fallback
- **THEN** the declared face is used
- **AND** the fallback applies only to families the document does not declare
