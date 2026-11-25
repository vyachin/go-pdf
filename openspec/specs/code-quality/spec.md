# code-quality Specification

## Purpose

Keeps the Go source consistently formatted and free of statically detectable
defects, and enforces that state on every change through continuous integration.

## Requirements

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

The contributor documentation, `CONTRIBUTING.md`, SHALL document how to install
the pinned linter, format the source, and run the same formatting and lint
checks locally. The README MUST NOT be the only place that records it.

#### Scenario: Contributor runs the checks

- **WHEN** a contributor follows `CONTRIBUTING.md`
- **THEN** they can format the source and run the same checks that CI runs

### Requirement: Automated action reference updates

The repository SHALL commit a Dependabot configuration that watches every action referenced by the workflows in `.github/workflows/` and checks for newer releases weekly.

#### Scenario: Weekly refresh

- **WHEN** a release of a referenced action is newer than the reference in the workflow
- **THEN** a pull request proposing the update appears within the weekly schedule

#### Scenario: Every referenced action is watched

- **WHEN** a workflow references an action
- **THEN** that reference is covered without adding a configuration entry per action

#### Scenario: A fresh release waits out the cooldown

- **WHEN** a release was published more recently than the configured cooldown
- **THEN** no pull request proposes it until the cooldown has elapsed

### Requirement: Updates grouped by change size

A dependency pull request SHALL bundle every minor and patch update of an action into one pull request, and MUST NOT bundle a major update with anything else.

#### Scenario: Routine updates arrive together

- **WHEN** two or more referenced actions have newer minor or patch releases
- **THEN** one pull request proposes all of them

#### Scenario: A major is alone

- **WHEN** a referenced action has a newer major version
- **THEN** its own pull request proposes that single major update

### Requirement: No automatic merging

A dependency pull request MUST NOT be merged automatically. The repository SHALL merge it only after the required checks pass and a human has reviewed the diff.

#### Scenario: Merge needs a person

- **WHEN** a dependency pull request passes every required check
- **THEN** it stays open until a human merges it

#### Scenario: No merge automation is configured

- **WHEN** the repository's workflows are reviewed
- **THEN** none of them enables auto-merge or approves a dependency pull request

### Requirement: Deliberate pins stay manual

Automation MUST NOT update the pins that need a human decision: the runner image and its system packages, the `pdfcpu` install version, the `golangci-lint` version, and the module's `go` directive.

#### Scenario: Golden rasterizer is untouched

- **WHEN** a dependency pull request changes an action reference
- **THEN** the runner image and the poppler package version in the workflow are unchanged

#### Scenario: Coupled tools are untouched

- **WHEN** a dependency pull request changes an action reference
- **THEN** `pdfcpu`, the linter version input and the `go` directive are unchanged

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

### Requirement: One action major per repository

The workflow SHALL reference a single major version of each action, and an update MUST move every reference to that action in the same pull request.

#### Scenario: Jobs agree on the major

- **WHEN** the workflow file is reviewed
- **THEN** the `test` and `lint` jobs reference the same major of `checkout` and the same major of `setup-go`

#### Scenario: An update moves every reference

- **WHEN** an action is updated
- **THEN** every job that references it is changed in the same pull request

### Requirement: Module dependencies watched before they exist

The configuration SHALL also watch the Go module, so that the first declared requirement is covered without a further configuration change.

#### Scenario: Nothing to report while the module is empty

- **WHEN** `go.mod` declares no requirements
- **THEN** the module watcher reports no updates and does not fail

#### Scenario: The first requirement is watched

- **WHEN** the module declares a requirement for the first time
- **THEN** the module watcher proposes its updates like any other dependency
