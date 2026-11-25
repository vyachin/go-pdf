# Spec Delta

## ADDED Requirements

### Requirement: The architecture reference matches repository boundaries

The contributor architecture reference SHALL name real repository packages and
describe their current responsibilities and control/data flow. It MUST identify
`pdfgen` as the public boundary, `internal/engine` as orchestration,
`internal/pdf` as the PDF writer, and `internal/backend/raster` as a debug consumer.
It MUST distinguish integer geometry storage from any floating-point scaling and
rounding actually used. Planned refactors MUST NOT be described as already shipped.

#### Scenario: A contributor follows the architecture diagram

- **WHEN** a reader follows a package path from the contributor diagram
- **THEN** the path exists and the named responsibility matches the code

#### Scenario: A reader checks numeric determinism

- **WHEN** the contributor guide explains millipoint geometry
- **THEN** it accurately states storage and rounding behavior without claiming no floating-point calculations occur

#### Scenario: A refactor is still proposed

- **WHEN** an architectural change has only planning artifacts
- **THEN** the current architecture guide does not describe that refactor as implemented
