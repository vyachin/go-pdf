# Spec Delta

## ADDED Requirements

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
