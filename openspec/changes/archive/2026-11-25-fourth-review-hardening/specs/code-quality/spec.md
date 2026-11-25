# Spec Delta

## ADDED Requirements

### Requirement: A test oracle compares what its documentation claims (RV4-45)

A test that claims to compare every outline SHALL exercise every font format the
engine accepts, including fonts converted from CFF, and MUST NOT silently skip a
glyph. A format or glyph the oracle cannot compare MUST be named in the claim.

#### Scenario: The sfnt oracle

- **WHEN** the documentation says the oracle compares every outline
- **THEN** the test covers OTTO fonts through the converted form
- **AND** a skipped glyph is reported rather than passed

### Requirement: The budget documents its concurrency contract (RV4-55)

The `Guard` documentation SHALL state whether it is safe for concurrent use. If it
is single-goroutine, the comment that describes a mutex MUST be removed and callers
MUST be told the guard belongs to one conversion.

#### Scenario: Reading the guard documentation

- **WHEN** a maintainer reads the package documentation
- **THEN** the concurrency contract is stated
- **AND** the description matches the implementation

### Requirement: Dead internal API is removed or wired (RV4-56)

An exported identifier under `internal/` that nothing calls SHALL be removed, and a
comment that names a removed consumer (the former CLI, the browser importer) MUST be
updated with the removal.

#### Scenario: A removed consumer

- **WHEN** the CLI or the importer that a comment names no longer exists
- **THEN** the comment does not claim it does
- **AND** helpers kept only for tests either move to a test file or gain a caller

### Requirement: The production path reads no test environment variable (RV4-57)

Runtime behaviour SHALL be selected through the API, not by an environment variable
read in the conversion path. A test override SHALL live behind a test seam, so a
deployment cannot change rendering by exporting a variable.

#### Scenario: A font override in a container

- **WHEN** a conversion runs with no font option set
- **THEN** the environment cannot select the font it uses
- **AND** the test harness still controls the font it needs

### Requirement: Shared test helpers have one definition (RV4-58)

A helper duplicated across test packages SHALL live in one internal test helper
package, so a change to the rule happens once and every suite follows it.

#### Scenario: The test font lookup

- **WHEN** a test package needs the bundled font
- **THEN** it calls the shared helper
- **AND** the search order is defined in one place

### Requirement: A diagnostic code is declared once (IMP-01)

Every diagnostic code SHALL be declared as a typed constant in one registry under
`internal/diag`, and every `Errorf`/`Warnf` call site MUST use a registered code.
The catalogue in the documentation SHALL be generated from the registry, and a test
MUST fail on an unregistered literal.

#### Scenario: A new code (IMP-01)

- **WHEN** a change adds a diagnostic
- **THEN** the code is added to the registry
- **AND** the catalogue reflects it
- **AND** a raw string literal fails the registry test

### Requirement: The gate includes a coverage floor and a mandatory validator (IMP-09)

The required gate SHALL include a coverage floor and run the independent PDF
validator on every job that writes a PDF, not only on the job that renders goldens.
A required check that is skipped MUST fail the gate.

#### Scenario: A job that writes PDFs (IMP-09)

- **WHEN** a job exercises the writer
- **THEN** it runs with the validator required
- **AND** coverage below the floor fails the gate

### Requirement: Test support code is shared and absent from production (IMP-10)

A helper used by more than one test package SHALL live in one internal test-support
package, and a test-only override MUST NOT be read by the production path.

#### Scenario: A second test package needs the font (IMP-10)

- **WHEN** another package needs the bundled test font
- **THEN** it imports the shared helper
- **AND** a conversion with no font option ignores the environment

### Requirement: A measured hot spot gets a fix and a benchmark (IMP-11)

A per-unit allocation or CPU cost identified by a review — line measurement, face
lookup, image decoding — SHALL be removed, and a benchmark MUST show the reduction
or guard against its return.

#### Scenario: A per-word allocation (IMP-11)

- **WHEN** the face lookup or the line measurement is fixed
- **THEN** the benchmark reports fewer allocations or less time
- **AND** the regression is visible in `go test -bench`

## MODIFIED Requirements

### Requirement: Rendering jobs share the pinned image

A job that rasterizes pages for a golden comparison SHALL run on the pinned runner
image with the pinned poppler package, and MUST NOT use a floating image label.
Moving either pin MUST regenerate the goldens in the same change. The workflow, the
renderer record committed beside the goldens and the documentation MUST name the same
image and the same renderer version.

#### Scenario: The pinned image renders

- **WHEN** the golden job runs
- **THEN** it uses the `ubuntu-24.04` image and the poppler package recorded in the workflow

#### Scenario: No floating label renders

- **WHEN** a workflow job rasterizes pages
- **THEN** its `runs-on` names the pinned image, not `ubuntu-latest`

#### Scenario: A pin moves

- **WHEN** the runner image or the poppler pin moves
- **THEN** the same change regenerates the render goldens

#### Scenario: The renderer record matches (RV4-29)

- **WHEN** the golden job runs with `PDF_REQUIRE_RENDER=1`
- **THEN** `testdata/golden/render/renderer.txt` names the renderer the job installs
- **AND** the documentation names the same image the workflow uses

#### Scenario: The documentation names the pin (RV4-29)

- **WHEN** a maintainer reads the CI table in `CONTRIBUTING.md` or `docs/golden.md`
- **THEN** the image it names is the one the workflow runs on
- **AND** a changed pin updates the workflow, `renderer.txt` and the documentation
  together
