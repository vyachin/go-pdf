# Design

## Context

See `proposal.md` — Why. The relevant state of the repository:

- `pdfgen` is a complete public package: `Convert`, `Validate`, `Options`,
  `Limits`, `Diagnostic(s)`, `Error`, `Result`, with external tests
  (`package pdfgen_test`) and two runnable examples. It imports nothing outside
  the standard library and the module's `internal/` tree.
- `cmd/pdfgen` does not use it. The command imports `internal/engine`,
  `internal/fsx` and `internal/profile`, calls `engine.Convert` directly, and
  owns its own input reading, atomic write and exit-code handling.
- `README.md` is converter-first: a capability tour, CLI flag tables, testing
  and linting instructions, performance figures and a milestone table. `docs/`
  mixes template-facing reference (`profile.md`, `fonts.md`,
  `browser-import.md`) with maintainer material (`golden.md`,
  `mpdf-corpus.md`).
- There is no `LICENSE`, no git tag and no `CHANGELOG.md`. `docs/profile.md` is
  read by `internal/profile/docs_test.go`, which keeps the documented display
  values honest.
- The lint configuration enables only the golangci-lint standard set, and
  `code-quality` forbids path, file and linter exclusions in it.

## Goals / Non-Goals

**Goals**

- A command that is a correct, minimal, compiling example of the public API.
- A hard, automated boundary between the example and the engine internals.
- Guarantees that the documentation states, each backed by a test.
- A documentation set where each document has one audience and one job.
- Adoption material: licence, version tag, changelog, stated compatibility.

**Non-Goals**

- No new exported API: no `fs.FS`, no reusable renderer, no streaming writer.
  All of these are additive later and none is justified without a consumer.
- No engine, profile, layout or PDF changes, so no golden snapshot moves.
- No second example program and no HTTP service.
- No move or rename of the user-facing `docs/` files, and no change to the
  `internal/profile/docs_test.go` contract on `docs/profile.md`.

## Decisions

### 1. The example calls `pdfgen` and nothing else

`cmd/pdfgen` is rewritten against `Convert`, `Options` and `*Error`. The one
thing it continues to own is policy: flag parsing, the default output path, the
write, and the exit code.

- *Alternative rejected:* keep `engine.Convert` and treat the CLI as an internal
  tool. Nothing would then demonstrate or test that the public API is sufficient.
- *Alternative rejected:* move the command into a separate module so the
  `internal/` import is impossible. It would break `go install
  github.com/vyachin/go-pdf/cmd/pdfgen@latest` and add a second `go.mod` for no
  benefit; the check below is cheaper.

### 2. The boundary is enforced by a direct-import check, not by the compiler

CI runs `go list -f '{{join .Imports "\n"}}' ./cmd/pdfgen` and fails when a line
contains `/internal/`. `go list .Imports` covers direct imports only, so
`pdfgen`'s own dependency on `internal/` — which is legitimate and hidden by Go
— does not trip the check.

- *Alternative rejected:* a `//go:build` tag or a custom lint rule. Both need
  more machinery for the same result.

### 3. No new exported symbols

The minimal example needs `Options{File, BaseDir, FontDir, ImportBrowser}`,
`Convert`, `Error` and `Diagnostics`; all exist. `DefaultLimits`,
`ParsePageSize`/`ParseMargin` and a structured profile export were candidates
while the command was assumed to expose every option; with a minimal command they
are speculative API and stay inside `internal/`.

### 4. Asset roots come from the input file, not from flags

`BaseDir` and `FontDir` are set to `filepath.Dir(input)`. A template that
references `logo.png` or `@font-face { src: url(font.ttf) }` beside itself then
works from any directory.

- *Alternative rejected:* flags for both roots. Two more settings on a command
  that must have one.
- *Alternative rejected:* the process working directory. It makes the result
  depend on where the user happened to stand, which is surprising and breaks the
  determinism story.

### 5. The example enables the browser import; the library default stays off

`ImportBrowser: true` is set by the command, with a comment saying it is the
example's policy. Browser-oriented templates are the common real input, and the
alternative is a first run that fails on `certificate.html` and points the reader
at a switch that no longer exists.

- *Consequence:* the command can no longer demonstrate strict mode or a
  profile-only conversion. Those are library-only now, documented in godoc and
  `docs/browser-import.md`.
- *Alternative rejected:* a `-strict` flag. That is the flag surface this change
  removes.

### 6. The example writes with `os.WriteFile` after a successful conversion

The write happens only after `Convert` returned a complete PDF; on any error
nothing is opened. The atomic-rename helper goes away with the rest of the
product code.

- *Trade-off:* a process crash or a full disk during the write can truncate the
  file. That is acceptable for an example: the library never writes, and making a
  write atomic is the caller's policy, stated once in the README.
- *Alternative rejected:* keep the atomic helper in the example. It is 20 lines
  of product behaviour that teaches nothing about the package.

### 7. Three guarantees become tests, and only then documentation

| Guarantee | Test | Mechanism |
|---|---|---|
| Concurrent conversions are safe | `TestConvertConcurrent` | goroutines converting the same and different templates; byte equality; CI already runs `go test -race` |
| Arbitrary input never panics | `FuzzConvert` | seeds from the corpus; asserts the call returns a result or an error and that a returned PDF parses as `%PDF` |
| A conversion writes nothing | `TestConvertWritesNothing` | `t.Chdir(t.TempDir())`, snapshot the directory, convert, snapshot again |

`t.Chdir` (Go 1.24+) is available under the module's Go 1.26 floor. `FuzzConvert`
uses small `Limits` so an iteration stays fast, and the CI fuzz step gains one
line next to the existing font fuzzers.

- *Alternative rejected:* documenting the guarantees without tests. The
  `library-docs` spec makes an untested claim a defect.

### 8. Documentation is split by audience, and files stay where they are

```
README.md                chooser: what / install / example / guarantees /
                         profile pointer / command / status / licence
doc.go + pdfgen docs     caller reference: package contract, every symbol
pdfgen examples          three concepts: Convert, Validate, *Error
docs/profile.md          template reference (read by docs_test.go)
docs/fonts.md            template reference
docs/browser-import.md   template reference
CONTRIBUTING.md          maintainer entry point: architecture, tests, goldens,
                         mPDF corpus, lint/format, performance, release
CHANGELOG.md, LICENSE    adoption
```

The package comment moves to `doc.go` so the top of pkg.go.dev is a contract, not
a preamble above the imports. `docs/golden.md` and `docs/mpdf-corpus.md` keep
their paths and are reached from `CONTRIBUTING.md`; moving them would churn
links for no gain.

- *Alternative rejected:* `docs/contributing/` as a directory. The split is
  about the entry point, not the depth of the tree.

### 9. The package comment states what the package is not

The new comment answers, in order: what it converts and for whom, what it
guarantees (determinism, no writes, no panics, concurrency, bounded work), what
it does not do (no browser CSS, no JavaScript, no network assets), the defaults,
and where the profile and the command are documented. It no longer claims that
the command is a thin wrapper, which is false today.

### 10. Versioning is pre-1.0 and says so

The first tag is `v0.1.0`. `CHANGELOG.md` starts with the tag and summarises the
milestones that were reached before it. The compatibility statement in the README
names as stable: diagnostic codes, `Options` fields (additive only), the
determinism guarantee and the error type. It names as unversioned: the reference
command, `internal/` packages and the breadth of the CSS profile.

- *Alternative rejected:* `v1.0.0` now. The profile will still grow; a v1 promise
  would freeze a contract before it has a single external consumer.

## Risks / Trade-offs

- **The library has no external consumer, so the claim "this is a good library"
  is unproven** → the example is a compile-and-test witness and the import check
  is enforced; the README will not claim adoption it does not have.
- **Removing CLI features is a regression for a hypothetical user** →
  `CHANGELOG.md` records the removal, and `reference-cli` states that the command
  carries no compatibility promise.
- **`FuzzConvert` can make CI slow or flaky** → fixed short fuzz time, small
  seeds, tight `Limits`; the target is also runnable on a corpus file.
- **The concurrency test can pass vacuously** → it asserts byte equality between
  parallel and sequential results, not merely the absence of a race report.
- **Packaging a first tag is irreversible in the module proxy** → tag only after
  `go test ./...`, `go test -race ./...`, the golden layers and `pdfcpu` pass on
  the exact commit.

## Migration Plan

1. Rewrite the command and its tests; add the import check to CI. The old
   subcommands stop working in the same change — no deprecation window, because
   the command never had a versioned surface.
2. Add the guarantee tests and the fuzz target; extend the CI fuzz step.
3. Rewrite README, package doc and examples; add `CONTRIBUTING.md`; delete the
   unreferenced `examples/` directory and move nothing else.
4. Add `LICENSE`, `CHANGELOG.md`, the compatibility statement, then tag `v0.1.0`
   and verify `go get github.com/vyachin/go-pdf/pdfgen@v0.1.0` resolves.

Rollback is a revert per step; steps 3 and 4 touch no code that the tests depend
on, and step 1–2 changes stay inside `cmd/` and `pdfgen/`.

## Open Questions

- The licence text is not chosen yet. MIT is proposed (permissive, the Go
  ecosystem norm), but the copyright holder must confirm it before the tag.
