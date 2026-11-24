# Spec Delta

## MODIFIED Requirements

### Requirement: Atomic result writing

The package SHALL NOT write the result: it returns the PDF to the caller. The reference command SHALL write the file only after a complete PDF was returned, so that a conversion that fails leaves the destination absent or unchanged and never partially written.

#### Scenario: Error during writing
- **WHEN** a conversion fails and the output path already holds a file
- **THEN** the target file is byte-identical to what it was before the run

#### Scenario: A successful conversion replaces the destination
- **WHEN** a conversion succeeds and the output path already holds a file
- **THEN** the destination holds the new PDF and no other file is created
