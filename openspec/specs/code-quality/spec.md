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
packages on every push to `main` and on every pull request. The gate MUST fail
when the linter reports an issue.

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
the test jobs, on every push to `main` and on every pull request. The test jobs
MUST NOT install or invoke the linter.

#### Scenario: Parallel feedback

- **WHEN** a push to `main` or a pull request triggers the workflow
- **THEN** the lint job runs independently of the test jobs and reports its own
  result

#### Scenario: Failing quality check

- **WHEN** the lint job fails on a pull request
- **THEN** the pull request shows a failing check for code quality

### Requirement: One workflow run per change

A change covered by a pull request SHALL be verified by that pull request's run
alone: a push to a branch other than `main` MUST NOT start a workflow run of its
own. A commit that lands on `main` MUST start exactly one run.

#### Scenario: A push to a feature branch

- **WHEN** a commit is pushed to a branch that has an open pull request
- **THEN** only the pull request run starts, and no `push` run appears for that
  commit

#### Scenario: A commit on the integration branch

- **WHEN** a commit lands on `main`, whether pushed directly or as a merged pull
  request
- **THEN** exactly one run starts for that commit

#### Scenario: A pull request against another base

- **WHEN** a pull request targets a branch other than `main`
- **THEN** its run still starts, because every pull request is verified

### Requirement: Trigger configuration is the only schedule

The workflow SHALL NOT start on a tag, a schedule or any event other than the
push to `main` and the pull request events, so that the set of runs matches the
set of changes under review or just landed.

#### Scenario: A tag is pushed

- **WHEN** a release tag is pushed to the repository
- **THEN** the workflow does not start; the commit it points at was already
  verified on `main`

#### Scenario: The configuration is read

- **WHEN** a maintainer reads the `on:` block of `.github/workflows/ci.yml`
- **THEN** it lists exactly the `push` restricted to `main` and the
  `pull_request` event

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

- **WHEN** a dependency pull request changes a dependency reference
- **THEN** the runner image and the poppler package version in the workflow are unchanged

#### Scenario: Coupled tools are untouched

- **WHEN** a dependency pull request changes a dependency reference
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

### Requirement: The Go project's implementation is preferred

Where the standard library or a `golang.org/x/...` module provides a facility the engine needs, the engine SHALL use it and MUST NOT commit a private substitute. A private implementation MAY remain only where the Go project's module does not cover the need, and each exception MUST be named with its reason in `CONTRIBUTING.md`.

#### Scenario: Normalization

- **WHEN** text must be brought to canonical composition
- **THEN** `golang.org/x/text/unicode/norm` performs it, and no composition table or generator is committed

#### Scenario: A JPEG header

- **WHEN** the writer needs the width, height or colour components of a JPEG
- **THEN** `image/jpeg` reports them, and no marker parser is committed

#### Scenario: UTF-16

- **WHEN** PDF strings, ToUnicode maps or extracted text are converted to or from UTF-16
- **THEN** `unicode/utf16` performs it, surrogate pairs included

#### Scenario: A justified exception

- **WHEN** a private implementation remains in the repository
- **THEN** `CONTRIBUTING.md` names the facility and why the Go project's module does not cover it

#### Scenario: The exception disappears

- **WHEN** a Go-project module starts covering a recorded exception
- **THEN** the change that adopts the module removes the private implementation

### Requirement: Test work runs as independent jobs behind one gate

The test work SHALL run as several jobs that start without waiting for each other, each reporting its own check. A gate job MUST depend on all of them, MUST fail when any is not successful — including a skipped one — and MUST be the single check branch protection requires.

#### Scenario: Jobs start together

- **WHEN** a workflow run starts
- **THEN** the unit, race, golden and fuzz jobs start without waiting for one another

#### Scenario: Every job reports its own result

- **WHEN** the run finishes
- **THEN** each job has its own check with its own result, and a failure names the job that failed

#### Scenario: A failure blocks the gate

- **WHEN** one job fails
- **THEN** the gate fails even if every other job succeeded

#### Scenario: A skip blocks the gate

- **WHEN** a job is skipped
- **THEN** the gate fails, because a suite that never ran is not a gate

#### Scenario: One required check

- **WHEN** branch protection is reviewed
- **THEN** it requires the gate, and `CONTRIBUTING.md` names that check

### Requirement: Elapsed time never fails the suite

The automated test suite MUST NOT fail because a wall-clock measurement exceeded a threshold; timings depend on the machine, the race detector and the runner load. Performance SHALL be reported by benchmarks and by the opt-in `PDF_PERF=1` report, which MUST NOT assert a threshold. Resource limits that count work — the `internal/budget` guard — and correctness checks SHALL remain.

#### Scenario: A slower machine

- **WHEN** the suite runs on a slower or loaded machine
- **THEN** no test fails because an elapsed-time threshold was exceeded

#### Scenario: Performance stays visible

- **WHEN** a contributor asks for the current numbers
- **THEN** the benchmarks and the `PDF_PERF=1` report log timings and allocations without failing on them

#### Scenario: Resource limits still apply

- **WHEN** a caller's operation budget is exhausted
- **THEN** the engine still reports the limit through its diagnostics and stops the work

#### Scenario: Correctness checks still fail

- **WHEN** a compared font table, a decoded position or a generated page diverges
- **THEN** the test fails as before, regardless of how long it took

### Requirement: Hostile-template fixtures run in the required gate

The unit and race jobs SHALL run the adversarial fixtures. The gate MUST fail when a
fixture is missing, skipped or ends without its expected diagnostic. The fixtures MUST
use the work budget and context cancellation as their pass criterion, consistent with
the ban on elapsed-time thresholds.

#### Scenario: A missing fixture fails the gate

- **WHEN** a fixture listed by the adversarial corpus is absent
- **THEN** the coverage test fails and names it

#### Scenario: A skipped fixture fails the gate

- **WHEN** a fixture skips because a tool is missing
- **THEN** the gate fails, as with the existing golden job

#### Scenario: The budget, not the clock

- **WHEN** a fixture runs on a slow machine
- **THEN** it still passes or fails on its diagnostic, not on elapsed time

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

### Requirement: Pushed history is never rewritten

A contributor MUST NOT rewrite history that has already been pushed: `git push
--force` and `git push --force-with-lease` are forbidden on every branch,
including a personal feature branch and the branch of an open pull request. A
correction is a new commit on top of the branch, and an unwanted commit is undone
with `git revert`, never erased. A released tag MUST NOT be moved, deleted or
force-pushed, and branch protection MUST reject a force push to `main`.

#### Scenario: A wrong commit on an open pull request

- **WHEN** a commit on a branch that is already pushed is wrong
- **THEN** the correction is a new commit on top of it
- **AND** the branch keeps the original commit instead of being rewritten

#### Scenario: Undoing a pushed change

- **WHEN** a pushed commit has to be undone
- **THEN** it is reverted with a new commit
- **AND** no force push rewrites the branch

#### Scenario: A released tag

- **WHEN** a tag `vX.Y.Z` has been pushed
- **THEN** the tag stays on the commit it names
- **AND** a fix ships as a new version, never as a moved tag
