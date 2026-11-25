# Spec Delta

## ADDED Requirements

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

