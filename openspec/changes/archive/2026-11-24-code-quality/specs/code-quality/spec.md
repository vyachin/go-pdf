# Spec Delta

## Purpose

Keeps the Go source consistently formatted and free of statically detectable
defects, and enforces that state on every change through continuous integration.

## ADDED Requirements

### Requirement: Canonical formatting

All tracked Go source files SHALL be formatted with `gofmt`. A file whose
formatting differs from `gofmt` output MUST fail the quality gate.

#### Scenario: Unformatted change

- **WHEN** a commit leaves a Go file in a form that `gofmt` would rewrite
- **THEN** the lint job fails and prints the affected files and their diffs

#### Scenario: Formatted tree

- **WHEN** every tracked Go source file matches `gofmt` output
- **THEN** the formatting step passes

### Requirement: Static analysis gate

The repository SHALL keep a committed linter configuration and run it over all
packages on every push and pull request. The gate MUST fail when the linter
reports an issue.

#### Scenario: New lint issue

- **WHEN** a change introduces a finding reported by the configured linter
- **THEN** the lint job fails and the finding names the file, position and check

#### Scenario: Clean tree

- **WHEN** the linter reports no findings for the whole module
- **THEN** the lint job succeeds

### Requirement: No blanket suppressions

The lint configuration MUST NOT exclude files, paths, linters or whole error
categories. Every deliberately ignored error SHALL be explicit at its call site,
and every suppression directive MUST name the affected check and the reason.

#### Scenario: Deliberately ignored error

- **WHEN** the code intentionally ignores a returned error
- **THEN** the discard is visible in the source at that call site rather than
  hidden by the configuration

#### Scenario: Configuration review

- **WHEN** the lint configuration is reviewed
- **THEN** it contains no preset, path or linter exclusions

### Requirement: Pinned, development-only toolchain

The linter version SHALL be pinned to one version in CI and in the documented
local setup. The linter MUST remain a development tool: it MUST NOT be added to
the module requirements, and no `go.sum` SHALL be introduced for it.

#### Scenario: Reproducible lint run

- **WHEN** CI runs the lint job
- **THEN** it uses the pinned linter version

#### Scenario: Module stays dependency-free

- **WHEN** the change is merged
- **THEN** `go.mod` is unchanged and the repository still has no `go.sum`

### Requirement: Separate CI quality job

Continuous integration SHALL run formatting and linting in a job separate from
the test job, on every push and pull request. The test job MUST NOT install or
invoke the linter.

#### Scenario: Parallel feedback

- **WHEN** a push or pull request triggers the workflow
- **THEN** the lint job runs independently of the test job and reports its own
  result

#### Scenario: Failing quality check

- **WHEN** the lint job fails on a pull request
- **THEN** the pull request shows a failing check for code quality

### Requirement: Documented local workflow

The README SHALL document how to install the pinned linter, format the source,
and run the same formatting and lint checks locally.

#### Scenario: Contributor runs the checks

- **WHEN** a contributor follows the README
- **THEN** they can format the source and run the same checks that CI runs
