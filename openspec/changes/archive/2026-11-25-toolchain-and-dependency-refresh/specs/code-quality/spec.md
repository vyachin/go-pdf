# Spec Delta

## ADDED Requirements

### Requirement: Coupled toolchain pins agree

The `pdfcpu` install version SHALL be buildable by the module's minimum Go version, which is the version `go.mod` selects in CI. A change that moves the `go` directive MUST move the `pdfcpu` pin in the same commit, or leave a pin the new minimum can still build.

#### Scenario: Directive and validator move together

- **WHEN** a change raises the `go` directive in `go.mod`
- **THEN** the same change installs a `pdfcpu` release whose own Go requirement the new minimum satisfies

#### Scenario: An unreachable validator is caught

- **WHEN** the workflow pins a `pdfcpu` release that declares a newer Go version than the module's minimum
- **THEN** the install step fails before the test job runs and the workflow reports a failure

#### Scenario: The validator still gates the output

- **WHEN** the `pdfcpu` pin moves
- **THEN** the test job still requires the validator through `PDF_REQUIRE_VALIDATOR=1` and validates in strict mode

## MODIFIED Requirements

### Requirement: Pinned, development-only toolchain

The linter version SHALL be pinned to one version in CI and in the documented
local setup. The linter MUST remain a development tool: it MUST NOT be added to
the module requirements, and no `go.sum` SHALL be introduced for it.

#### Scenario: Reproducible lint run

- **WHEN** CI runs the lint job
- **THEN** it uses the pinned linter version

#### Scenario: Module stays dependency-free

- **WHEN** a change lands that does not deliberately move the `go` directive
- **THEN** `go.mod` gains no requirements and the repository still has no `go.sum`

#### Scenario: The directive moves on purpose

- **WHEN** the `go` directive in `go.mod` differs from the previous revision
- **THEN** the same change states the new minimum wherever the workflow and the README record it, and no requirement or `go.sum` entry appears
