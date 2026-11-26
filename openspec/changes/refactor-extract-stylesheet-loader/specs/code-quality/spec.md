# Spec Delta

## ADDED Requirements

### Requirement: Validation does not read assets (RV5-50)

`internal/profile` SHALL validate parsed documents, stylesheets, declarations
and `@font-face` descriptors only; it MUST NOT import asset or filesystem
packages, and MUST NOT open, stat or decode a file. Loading and decoding SHALL
move to a separate `internal/stylesheet` stage that resolves each href once
through the asset registry and preserves every existing `css/link-*` code,
message and position.

#### Scenario: The profile package imports no asset code

- **WHEN** the production imports of `internal/profile` are inspected
- **THEN** none of `internal/assets`, `internal/fsx`, `internal/asset`, `os` or
  `path/filepath` appears
- **AND** the import-graph test fails if one is added later

#### Scenario: A linked stylesheet is still diagnosed the same way

- **WHEN** a `<link rel="stylesheet">` names a missing file, a remote URL, a
  `data:` URI or a path that escapes the base directory
- **THEN** the same `css/link-*` code, message and position are reported as
  before the extraction

#### Scenario: A reference is resolved once

- **WHEN** an href carries a query, a fragment, percent-encoding or surrounding
  whitespace
- **THEN** one resolver normalizes it and the registry reads the resulting file
- **AND** the loader does not pre-resolve the same reference a second time

#### Scenario: A legacy stylesheet keeps its positions

- **WHEN** a linked sheet is decoded from windows-1251 or carries an `@charset`
  rule
- **THEN** every diagnostic points at the same line and column as before
- **AND** the parsed rules and their offsets are unchanged
