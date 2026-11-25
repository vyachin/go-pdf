# Spec Delta

## ADDED Requirements

### Requirement: Automated Go module updates

The committed Dependabot configuration SHALL watch every requirement in `go.mod` weekly, with a cooldown for a fresh release, and open a pull request when a required module has a newer version. A module MUST be covered by this configuration without an entry per module.

#### Scenario: A newer module release

- **WHEN** a required module has a release newer than the version pinned in `go.mod`
- **THEN** a pull request proposing it appears within the weekly schedule

#### Scenario: Every requirement is watched

- **WHEN** `go.mod` requires a module
- **THEN** that module is covered without adding a configuration entry per module

#### Scenario: A fresh release waits out the cooldown

- **WHEN** a module release was published more recently than the configured cooldown
- **THEN** no pull request proposes it until the cooldown has elapsed

### Requirement: A module update is a deliberate change

A module update pull request MUST refresh `go.sum`, MUST leave the `go` directive unchanged, and MUST NOT add or remove a module requirement; a release that renames the module path is therefore a human change. A minor or major update SHALL arrive alone; only patch updates MAY be grouped.

#### Scenario: Checksums move with the version

- **WHEN** a module update pull request bumps a version
- **THEN** `go.mod` and `go.sum` change in the same pull request and `go mod verify` passes

#### Scenario: The go directive stays

- **WHEN** a module update pull request is opened
- **THEN** the `go` directive in `go.mod` is unchanged

#### Scenario: A release that renames the module path

- **WHEN** a module publishes a major release under a new module path
- **THEN** no automatic pull request replaces the requirement; the change is made by hand as a `code-quality` decision

#### Scenario: Patch updates travel together

- **WHEN** two required modules have newer patch releases
- **THEN** one pull request proposes both

#### Scenario: A minor update is alone

- **WHEN** a required module has a newer minor version
- **THEN** its own pull request proposes it, because a module below v1 may change its API in a minor release

#### Scenario: The allowlist still holds

- **WHEN** a module update pull request is opened
- **THEN** every module in the build list remains one the Go project maintains

## MODIFIED Requirements

### Requirement: Deliberate pins stay manual

Automation MUST NOT update the pins that need a human decision: the runner image and its system packages, the `pdfcpu` install version, the `golangci-lint` version, and the module's `go` directive.

#### Scenario: Golden rasterizer is untouched

- **WHEN** a dependency pull request changes a dependency reference
- **THEN** the runner image and the poppler package version in the workflow are unchanged

#### Scenario: Coupled tools are untouched

- **WHEN** a dependency pull request changes a dependency reference
- **THEN** `pdfcpu`, the linter version input and the `go` directive are unchanged
