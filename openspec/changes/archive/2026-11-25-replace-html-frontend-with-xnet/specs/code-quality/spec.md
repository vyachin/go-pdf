# Spec Delta

## REMOVED Requirements

### Requirement: Pinned, development-only toolchain

**Reason**: The requirement combined the linter pinning rule with a rule that the
module must stay dependency-free. The dependency-free scenarios are replaced by
the dependency allowlist below; the linter rule survives as its own requirement.

**Migration**: The linter pinning and the "the `go` directive moves on purpose"
scenarios move to "Pinned linter toolchain" and "Dependency allowlist". The
guidance that `go.mod` gains no requirements and no `go.sum` exists is deleted:
`go.sum` is now expected whenever a requirement exists.

## ADDED Requirements

### Requirement: Pinned linter toolchain

The linter version SHALL be pinned to one version in CI and in the documented
local setup. The linter MUST remain a development tool: it MUST NOT be added to
the module requirements, and no `go.sum` SHALL be introduced for it.

#### Scenario: Reproducible lint run

- **WHEN** CI runs the lint job
- **THEN** it uses the pinned linter version

### Requirement: Dependency allowlist

A module requirement SHALL be allowed only for a module maintained by the Go
project (`golang.org/x/...`); the standard library needs no requirement. An
allowed module MUST be pinned to a released version, and its checksum MUST be
committed in `go.sum`. A dependency maintained outside the Go project MUST
change this capability before it lands.

#### Scenario: A Go-project module is required

- **WHEN** a change makes the module depend on a `golang.org/x/...` package
- **THEN** `go.mod` names a released version and `go.sum` carries its checksum

#### Scenario: A dependency outside the Go project

- **WHEN** a change proposes a module the Go project does not maintain
- **THEN** that change modifies this capability, and the dependency lands in a
  later change

#### Scenario: A requirement is removed

- **WHEN** the last import of a required module disappears
- **THEN** the same change drops it from `go.mod` and `go.sum`

#### Scenario: The directive moves on purpose

- **WHEN** the `go` directive in `go.mod` differs from the previous revision
- **THEN** the same change states the new minimum wherever the workflow and the
  README record it
