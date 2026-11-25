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

### Requirement: A breaking change is released as a major version (RV4-08)

A release that removes or renames an exported identifier SHALL be a major version,
and `CHANGELOG.md` MUST carry a migration section for it. The README MUST state to
which released version the documented API belongs, so a reader is not told about an
API that `go get` does not provide.

#### Scenario: Removing an exported option

- **WHEN** the Unreleased section removes a field from `Options`
- **THEN** the release is a major version
- **AND** the changelog says how a caller migrates
- **AND** the README names the released version its example compiles against

### Requirement: The diagnostic catalogue matches the code (RV4-25)

The profile document SHALL list every diagnostic code the code can emit, and a test
MUST fail when a code is emitted but undocumented or documented but not emitted.
The catalogue MUST be fit for matching, because codes are the stable surface.

#### Scenario: A new code

- **WHEN** a change introduces a diagnostic code
- **THEN** the catalogue test fails until the code is documented
- **AND** removing a documented code fails the same test

### Requirement: The README describes only current capabilities (RV4-26)

The README MUST NOT advertise a capability the code no longer has. Removing a
feature SHALL update the README in the same change, as it updates the changelog.

#### Scenario: A removed compatibility layer

- **WHEN** the browser-lowering layer is removed
- **THEN** the README says a browser-oriented template must be rewritten within the
  profile
- **AND** no document offers lowering as a supported path

### Requirement: The profile document states what the code enforces (RV4-27)

Each generated-content position SHALL be documented with the values it actually
accepts, and the position rule for diagnostics documented for one class SHALL match
the code for every class.

#### Scenario: Generated content

- **WHEN** the profile document describes `a::after` and the page margin boxes
- **THEN** the documented values are exactly the ones validation accepts
- **AND** a document using the documented form validates

### Requirement: The font directory fallback is documented (RV4-60)

`Options.FontDir` documentation SHALL say that a relative `@font-face` path resolves
against `FontDir` and otherwise against `BaseDir`, and which diagnostic reports an
unusable root.

#### Scenario: No FontDir but a BaseDir

- **WHEN** a caller sets `BaseDir` and leaves `FontDir` empty
- **THEN** the documented rule explains that `@font-face` files resolve inside
  `BaseDir`
- **AND** a root that cannot be opened is documented as `options/asset-root`

### Requirement: Documentation formatting renders as written (RV4-63)

A list MUST NOT be interrupted by an unrelated paragraph, a code fence MUST contain
only commands, and prose about a removed flag MUST be updated with the removal.

#### Scenario: A commands block

- **WHEN** a documentation block is labelled as commands
- **THEN** every line in it is a command
- **AND** explanatory prose sits outside the fence

### Requirement: Open work has one recorded home (RV4-64)

Documented planned work SHALL be represented by an OpenSpec change or by a
known-gaps section that names an owner. A statement that open work is tracked in one
place MUST be true of the repository at that commit.

#### Scenario: A documented gap

- **WHEN** a document lists a feature as planned or worth closing
- **THEN** an active change or a known-gaps entry references it
- **AND** the statement about where open work lives stays accurate

### Requirement: Performance figures are current or clearly indicative (RV4-65)

A performance number in contributor documentation SHALL be refreshed with the
release it describes, or labelled indicative together with the command that measures
it.

#### Scenario: The benchmark table

- **WHEN** a reader opens the performance section
- **THEN** the figures match a recent run or are marked indicative
- **AND** the command that reproduces them is given

### Requirement: Documents that state code facts are generated or checked (IMP-06)

A document that states a limit default, a diagnostic code, the CI image or the
renderer version SHALL derive that fact from the code or the workflow, or a test
MUST fail when the two disagree.

#### Scenario: A default changes (IMP-06)

- **WHEN** a limit default changes
- **THEN** the documentation test fails until the table is updated or regenerated
- **AND** the same holds for the diagnostic catalogue and the CI image

### Requirement: Reproducibility documentation names every input

Every consumer-facing byte-reproducibility guarantee SHALL state or directly
reference its prerequisites: identical engine build, HTML bytes, options, resolved
stylesheet/image/font bytes and resolution choices. It MUST recommend explicit
fonts and immutable resources for reproducible deployments and explain that the
fixed default creation date removes clock dependence. System-font fallback MUST
remain documented as environment-dependent convenience.

#### Scenario: A reader evaluates the README guarantee

- **WHEN** the reader encounters the deterministic-output guarantee
- **THEN** its complete-input prerequisites are visible or directly linked without requiring contributor documentation

#### Scenario: A deployment requires repeatable documents

- **WHEN** a caller follows the font and resource guidance
- **THEN** the guidance uses explicitly selected font bytes and stable stylesheet/image resources rather than assuming system fallback is identical everywhere

### Requirement: Diagnostic documentation states location availability

The diagnostic reference SHALL distinguish known source positions, option errors
and genuinely aggregate/output failures. It MUST explain that zero line or column
means unknown, that an element path is supplied only when an owning element is
known, and that linked stylesheet diagnostics use the stylesheet file. It MUST NOT
promise complete source coordinates for every error, or excuse missing coordinates
when the originating source location is available.

#### Scenario: A caller handles an aggregate diagnostic

- **WHEN** the caller reads a document-named diagnostic with zero line and column
- **THEN** the documentation explains those fields as unknown and shows how to format the diagnostic without invented coordinates

#### Scenario: A source declaration fails

- **WHEN** a stylesheet declaration has a known source position
- **THEN** the reference promises its actual source file and position rather than the generic aggregate fallback

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
