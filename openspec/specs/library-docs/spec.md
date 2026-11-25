# library-docs Specification

## Purpose

Defines documentation as a deliverable of the library: who each document serves,
what the README and the package documentation must answer, how examples are
limited, where contributor material lives, and which material a consumer needs
before adopting the package.

## Requirements

### Requirement: The README is a front page for a chooser

The README SHALL answer, in this order: what the package does, how to install and
call it in a short example, the guarantees it makes, a pointer to the supported
CSS, how to run the reference command, and the project's status and licence. It
MUST NOT carry contributor instructions, snapshot-regeneration procedures or a
tour of every capability.

#### Scenario: A working example is within the first screen

- **WHEN** a reader opens the README
- **THEN** an install command and a complete conversion example appear before any reference material

#### Scenario: Contributor material is absent

- **WHEN** the README is reviewed
- **THEN** it contains no formatting, linting, testing or golden-snapshot instructions

### Requirement: The package documentation is the caller's reference

Every exported identifier SHALL carry a doc comment that states its contract, its
defaults and its error behaviour, rather than restating its name. The package
comment MUST state what the package does not do, the guarantees listed in
`library-api`, and the resource limits.

#### Scenario: Every exported identifier is documented

- **WHEN** the exported-comment check runs over the package in the test suite
- **THEN** it reports no undocumented exported identifier or package

#### Scenario: `go doc` answers the caller

- **WHEN** a caller runs `go doc` on the package, `Options`, `Result` and `Error`
- **THEN** each shows what is guaranteed and what is left to the caller

### Requirement: Examples document one concept each

The package SHALL provide runnable examples for the three decisions a caller
makes: converting a document, validating one, and handling an `*Error`. An
example MUST NOT be used to tour capabilities; a new example is justified only by
a caller decision that none of the existing examples demonstrates.

#### Scenario: Examples are executable documentation

- **WHEN** the package tests run
- **THEN** every example compiles as part of the test suite, and an example that states its output runs and passes

#### Scenario: An example shows one decision

- **WHEN** a reader opens an example
- **THEN** it shows one decision end to end, without a catalogue of options

### Requirement: Contributor material has its own entry point

Architecture, test layers, golden snapshots, the mPDF corpus, the WeasyPrint
samples corpus, formatting, linting, performance measurement and the release
procedure SHALL be documented under `CONTRIBUTING.md`, directly or through
links, and MUST NOT be required reading for a consumer.

#### Scenario: A maintainer finds every workflow

- **WHEN** a maintainer opens `CONTRIBUTING.md`
- **THEN** it links to each workflow from a single page

#### Scenario: The README does not duplicate it

- **WHEN** contributor instructions change
- **THEN** only `CONTRIBUTING.md` and the documents it links need updating

### Requirement: Adoption material states licence and versioning

The repository SHALL declare a licence, tag released versions, keep a
`CHANGELOG.md`, and state the versioning policy: which surface is stable across
minor versions and which may change. The policy MUST name diagnostic codes,
`Options` fields and deterministic output as stable, and the reference command
and `internal/` packages as unversioned.

#### Scenario: A consumer can judge the risk

- **WHEN** a consumer evaluates the package
- **THEN** the repository states the licence, the released version and what a minor upgrade may change

#### Scenario: A release is recorded

- **WHEN** a version is tagged
- **THEN** `CHANGELOG.md` describes that version and the tag resolves for `go get`

### Requirement: Documented claims are enforced by tests

A guarantee or default stated in documentation SHALL be enforced by a test. A
documented default value MUST equal the value the code uses, and a documented
guarantee that no test can enforce MUST be removed instead of kept as prose.

#### Scenario: A documented default is asserted

- **WHEN** the documentation states a default page size, margin or limit
- **THEN** a test asserts that default

#### Scenario: An unsupported claim is removed

- **WHEN** a guarantee has no test behind it
- **THEN** either the test is added in the same change or the claim is deleted
