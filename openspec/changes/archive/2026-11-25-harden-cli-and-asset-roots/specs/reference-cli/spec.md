# Spec Delta

## ADDED Requirements

### Requirement: The output never overwrites the input

The command SHALL refuse to write when the resolved output path and the input path
name the same file, including a case-insensitive filesystem, and MUST exit with a
usage or failure code instead of truncating the source. On success it SHALL write
through a temporary file in the destination directory and rename it into place, so a
failed write cannot leave a partial file at the output path.

#### Scenario: The input is named template.pdf

- **WHEN** `pdfgen template.pdf` runs and the default output equals the input
- **THEN** the command exits non-zero and the input is unchanged

#### Scenario: An explicit output equal to the input

- **WHEN** `pdfgen -o invoice.html invoice.html` runs
- **THEN** the command refuses and the source file is unchanged

#### Scenario: A failed write preserves the previous file

- **WHEN** the conversion succeeds but the rename or write fails
- **THEN** the destination still holds its previous bytes

### Requirement: The input is size-checked before reading

The command SHALL reject an input larger than the default `MaxInputBytes` before
loading it, using the limits the package exports, and SHALL report the same
`resource/limit` message the package uses.

#### Scenario: An oversized input

- **WHEN** the input file exceeds the default limit
- **THEN** the command exits with the failure code and reports the limit
- **AND** it does not read the whole file into memory
