# Design

## Context

See [proposal.md](proposal.md) for motivation and
[specs/code-quality/spec.md](specs/code-quality/spec.md) for the requirements.

The module targets Go 1.24 and has no external dependencies, so it has no
`go.sum`. CI runs one `test` job with `go vet`, tests, race tests, short fuzz
runs and a pdfcpu-based validation; it installs poppler and pdfcpu, which makes
it slower than a pure analysis job. The source is already `gofmt`-clean, and a
full `golangci-lint` run (per-message display limits lifted) reports 72 findings
in 22 files: 49 `errcheck` (18 CLI writes, 9 production closes/removes, 22 test
closes), 12 `staticcheck` (8 quickfix, 1 `S1008`, 3 `SA1019`), 9 `unused`,
2 `ineffassign`.

## Goals / Non-Goals

**Goals:**

- One pinned analysis tool that runs locally and in CI with the same result.
- Formatting enforced on every change without adding a second formatter.
- Every ignored error visible at its call site, not hidden in configuration.
- The module stays dependency-free and keeps no `go.sum`.

**Non-Goals:**

- Enabling additional linters (`revive`, `gosec`, `gocritic`, …) or complexity
  limits. They would add findings that this change does not budget for.
- Pre-commit hooks or a Makefile. The README commands are enough for a
  single-maintainer repository; either can be added later without touching the
  specs.
- Changing product behaviour. The source cleanup is limited to how errors are
  reported inside the code and to deleting unreachable code.

## Decisions

### 1. `golangci-lint` over standalone `go vet` + `staticcheck`

`golangci-lint` v2 bundles `govet`, `staticcheck`, `errcheck`, `ineffassign`
and `unused`, gives one configuration and one pinned version, and annotates pull
requests through its official action. The locally installed standalone
`staticcheck` 2025.1.1 cannot read Go 1.27 export data, while the `staticcheck`
inside `golangci-lint` 2.15 works with the current toolchain, so mixing two
tools would also mix two compatibility stories.

*Alternatives*: `go vet` plus `staticcheck` (two tools to pin, no `errcheck` or
`unused`); a linter framework such as `revive` alone (misses type-based checks).
`golangci-lint` covers the needed checks with the least tooling.

### 2. Standard linter set, no presets and no exclusions

The configuration enables the `standard` set and nothing else:

```yaml
version: "2"
linters:
  default: standard
```

The default `std-error-handling` preset (formerly part of the v1 defaults)
would remove all 48 `Close`/`Remove`/`fmt.Fprint` findings at once. The change
deliberately does not enable it: the project chose to make each discard a
reviewable line of code. The cost is 49 edited call sites now and an explicit
`_ =` whenever future code closes a read-only resource.

*Trade-off*: the diff is larger than with the preset, and the gate is stricter
than the Go ecosystem default. The `writeFileAtomic` path keeps its checked
`f.Close()`; the linter is not the only protection there.

### 3. `gofmt` is the formatter

`gofmt -l` is clean on all tracked files and `gofmt -s` changes nothing. The
module imports only the standard library, so `goimports` would add a tool
without adding value (there are no import groups to separate). `gofumpt` would
rewrite exactly one line (`var a, b = -1, -1` → `:=`) and impose a
non-standard style for no correctness benefit.

Formatting is checked in a shell step, not through `golangci-lint fmt`, because
the action runs `golangci-lint run` only and `fmt --diff` exits successfully
even when it prints a diff. The shell guard (`gofmt -l`, print, `exit 1`) is
smaller and cannot silently pass. Locally the documented command uses
`git ls-files '*.go'` so the ignored `.cache/` directory is not scanned; CI
checks a clean checkout and can use `.`.

### 4. Explicit discards, `check-blank` stays off

`errcheck` defaults to reporting only errors that are neither used nor assigned.
Keeping `check-blank: false` makes `_, _ = fmt.Fprintf(...)` and
`defer func() { _ = f.Close() }()` the accepted way to discard an error
deliberately. Enabling `check-blank` would reject those and force `//nolint`
comments instead, which is the opposite of the visibility goal.

Where an error can affect the result (writing the PDF, closing after a write),
it is returned. Where it cannot (closing a read-only handle, writing help text
to a stream), the discard is explicit.

### 5. `SA1019`: `go/build.Default.GOROOT` in tests

Three tests locate the CMYK JPEG fixture in the Go distribution through
`runtime.GOROOT()`, which is deprecated because a binary moved to another
machine keeps a stale root. Test binaries are not distributed, so the premise
does not apply. `go/build.Default.GOROOT` is the non-deprecated API that exposes
the same value without a `//nolint` and without spawning `go env GOROOT` from
the test.

*Alternatives*: a `go env GOROOT` test helper (the documented replacement, but
adds a subprocess and a PATH dependency); copying the 19 KB fixture into
`testdata/` (removes the dependency but adds a binary whose provenance is
unclear); `//nolint:staticcheck` (suppresses instead of fixing).

### 6. Dead code is removed

`unused` analyzes test files as well, so the nine findings are declarations
with no users at all: `fillPolygon`, `parseInt`, `parseIdent`, `attrInt`, the
`y` and `node` fields, `htmlNode`, `checkPageBreak`, and the `codes` test
helper. They are deleted. The test-only `(*kerning).parsePairPos` wrapper in
`internal/font/gpos.go` is not reported (tests call it), so it stays as is to
keep this change scoped to the gate. The stale `//nolint:staticcheck` in
`internal/layout/pagination_test.go` is removed as part of the same cleanup.

The two `ineffassign` findings are not deleted blindly:
`internal/layout/layout_test.go` appends validation diagnostics that the harness
then drops (likely a real loss), and `internal/layout/lines.go` assigns
`canWrap = false` that is never read (possibly a latent double-wrap). The task
investigates the intended behaviour before editing.

### 7. CI: one extra job, versions pinned

A `lint` job runs on the same `push`/`pull_request` triggers as `test`:

1. checkout and `actions/setup-go` from `go.mod` (no module cache: no `go.sum`);
2. the `gofmt` guard;
3. `golangci/golangci-lint-action@v9.3.0` with `version: v2.15.0`.

It installs neither poppler nor pdfcpu, so it stays fast and its result is
independent of external tools. The `test` job is not modified. Making `lint` a
required check is a GitHub branch-protection setting, documented in the change
but not stored in the repository.

*Why pin both*: the linter action and binary change behaviour between releases.
`golangci-lint` v2.15 is built with Go 1.26 and later, which is why the README
points at the prebuilt binary rather than `go install` under the module's
Go 1.24 minimum.

### 8. README instead of a task runner

The project has no Makefile today and its commands are short. The README gains
a "Formatting and linting" section next to "Testing" with the install command,
`gofmt -w $(git ls-files '*.go')` and `golangci-lint run ./...`. A Makefile or
pre-commit hook can be added later without changing the specification.

## Risks / Trade-offs

- **Strict `errcheck` becomes noisy** for every new `defer f.Close()` →
  documented policy: check the error or discard it explicitly with `_ =`;
  revisit the preset only through a deliberate change.
- **A linter upgrade needs a newer Go toolchain to build** → CI and the README
  use the pinned prebuilt binary; the module's `go 1.24` minimum is unaffected.
- **Removing "unused" code removes something needed later** → every deletion is
  verified by the test suite and by a full-package compile; the one test-only
  helper that the linter does not report is left untouched.
- **The cleanup diff spans 22 files** → commits are split by category
  (`errcheck`, dead code, `staticcheck`, CI, docs) so each is reviewable.
- **`gofmt -l .` scans ignored directories locally** → the documented local
  command lists tracked files; CI runs on a clean checkout.
- **`build.Default.GOROOT` still bakes the build-time root** → the affected
  tests already skip when the fixture is absent, so a stale path degrades to a
  skip, not a failure.

## Migration Plan

1. Add `.golangci.yml` and fix the source in category-sized commits.
2. Verify locally with `gofmt -l`, `golangci-lint run ./...`, `go test ./...`
   and `go test -race ./...`.
3. Add the `lint` job last so the pull request shows a green gate.
4. After merge, mark the `lint` check as required in branch protection.

Rollback is reverting the workflow and configuration files; the source cleanup
is behaviour-preserving and can stay.
