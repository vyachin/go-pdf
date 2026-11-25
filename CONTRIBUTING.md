# Contributing

This file is the entry point for working on the engine. The consumer-facing
documentation is the README; nothing here is required reading to use the package.

## Layout

```
  HTML bytes
     |
     v
 internal/html      scan (positions, limits) + standard tree + model
     |
     v
 internal/css       tokenizer, parser, selectors, cascade
     |
     v
 internal/profile   allow-list, validation, diagnostics
     |
     v
 internal/engine    orchestration: prepare, layout, write; one work guard
     |        \
     |         +-> internal/assets   confined reads, identity, accounting
     v
 internal/layout    box tree -> lines -> fragments -> pages
     |        \
     |         +-> internal/backend/raster   debug raster (tests only)
     v
 internal/text      shaping, line breaking, metrics
     |
     v
 internal/pdf       PDF writer, fonts, ToUnicode
```

`pdfgen` is the only public package; every `internal/...` package is an
implementation detail that Go hides from other modules. `cmd/pdfgen` is the
reference example and must stay one: it imports `pdfgen` and the standard
library only, because the command's job is to prove the public API is enough.

Who owns what:

- `internal/engine` orchestrates one conversion: it prepares the document, owns
  the single `budget.Guard` every stage charges, runs layout, and hands the
  layout's drawing operations to the writer. It creates the `internal/assets`
  registry and closes it on every exit, so a cancelled or failed conversion
  releases every file handle.
- `internal/assets` owns resource access: the confined readers (`internal/fsx`
  roots), the identity of an asset (kind, canonical file or content digest, and
  decoding inputs), the conversion's admission counters, and typed failures.
  `internal/asset` is the URL-path resolver it uses.
- `internal/layout` owns geometry and page composition: the box tree, line
  breaking, fragments, pages, and the drawing operations the writer serialises.
  `internal/text` owns shaping, metrics and the glyph scaling it needs.
- `internal/pdf` owns serialisation only: objects, the `Identity-H` font,
  `ToUnicode` and number formatting. It does not decide layout.
- `internal/docdump` dumps the prepared model for the document snapshots, and
  `internal/backend/raster` rasterises layout's drawing operations for
  `Options.RenderPNG` and the render goldens. Both are debugging and test
  consumers of what layout already produced; neither is a prerequisite for
  writing the PDF.

Geometry is stored in integer millipoints (1/1000 pt): `int32` positions, sizes
and advances. Not every calculation is integer — `text.Scale` converts font
units to millipoints with `math.Round` over `float64`, and the writer scales
font descriptors and nominal widths the same way. The determinism guarantee
comes from identical inputs and a fixed rounding rule, not from avoiding
floating point entirely.

## Standard library first

Where the standard library or a `golang.org/x/...` module provides a facility
the engine needs, the engine uses it; a private implementation stays only where
no such module covers the need, and then it is listed here with its reason. The
`code-quality` requirement in the OpenSpec specs makes this rule durable, so
review both directions: a new hand-written substitute, and a listed exception
that a module now covers.

- **Font subsetter** (`internal/font`). `x/image/font/sfnt` parses `cmap`,
  `hmtx` and `glyf`, but it does not expose the raw tables the subsetter
  rewrites and it cannot subset. Adopting it would keep the private parser and
  add a second one, so it stays a test oracle instead: `sfnt_oracle_test.go`
  compares the character map, the advances and the vertical metrics of every
  corpus font, the outline of every glyph segment by segment for TrueType, and
  the outline anchors of a converted CFF font, and `sfnt_subset_test.go` hands
  it the subsetter's rewritten tables. A glyph the oracle cannot compare —
  coloured, missing, or a charstring whose operators sfnt rejects — is counted
  and logged, never skipped silently. Kerning is not compared — `sfnt` reads a
  Latin-only subset of `GPOS`.
- **Debug raster scaler** (`internal/backend/raster`). Nearest-neighbour
  scaling is deliberate: `testdata/golden/render` is compared pixel by pixel,
  and any interpolation would make every golden depend on the filter.
- **CSS grammar** (`internal/css`). No Go-project module implements the CSS
  tokenizer, parser, selectors or cascade.
- **PDF syntax** (`internal/pdf`). No Go-project module writes the objects, the
  `Identity-H` font, the `ToUnicode` map or the number formatting the output is
  pinned to.

## Tests

```bash
go test -shuffle=on ./...                           # unit and integration tests
go test -shuffle=on -race ./...                     # the concurrency guarantee
PDF_REQUIRE_VALIDATOR=1 go test -shuffle=on ./...   # require pdfcpu, do not skip
go test ./internal/engine -bench .                  # benchmarks
```

- The suite runs with `-shuffle=on`, so each package's tests run in a random
  order. A test that depends on another test having run first fails here and in
  CI instead of staying a latent assumption; fix the test, not the flag.
- Document snapshots (`testdata/golden/xml`) freeze what the engine understood:
  elements, effective styles, the `@page` setup and the diagnostics, as
  canonical XML. They need no font and no external tool, so a failure is a
  parser, cascade or profile change.
- Render snapshots (`testdata/golden/render`) freeze every page as a 72 dpi PNG
  and compare pixels exactly. They need poppler and are produced with the
  poppler version recorded next to the images. Format, scope and the update
  procedure are in [docs/golden.md](docs/golden.md).
- Generated PDFs are validated by the independent `pdfcpu` tool when it is in
  `PATH`; CI requires it through `PDF_REQUIRE_VALIDATOR=1`.
- The `testdata/*.html` corpus holds an invoice, an act and a contract with real
  layout; `testdata/fixtures/` adds one small document per profile feature. Four
  of those fixtures are not UTF-8 on purpose: `cp1251-invoice.html`,
  `koi8r-invoice.html`, `utf8-bom-invoice.html` and the `cp1251-styles.css` the
  invoice links. Their bytes are what the encoding tests read, so
  `.gitattributes` marks them `-text`; never normalise or "fix" them.
- The mPDF corpus (`internal/mpdfcompat`) contains 149 documents extracted from
  the mPDF tests. It checks that a document inside the profile yields a valid
  deterministic PDF, while a document outside the profile is rejected with
  diagnostics instead of a crash. Details are in
  [docs/mpdf-corpus.md](docs/mpdf-corpus.md).
- The WeasyPrint corpus (`internal/weasycompat`) holds six full documents from
  `CourtBouillon/weasyprint-samples` with the PDFs WeasyPrint produced. Each case
  declares whether the profile rejects it, whether it converts, or whether it
  matches the reference text, and the suite enforces that declaration; the
  current distance from the profile is in
  [docs/weasyprint-corpus.md](docs/weasyprint-corpus.md).
- Fuzz targets cover the HTML frontend, the CSS tokenizer, TrueType parsing,
  the PDF writer and the public `Convert` boundary (arbitrary bytes never panic).

### Adversarial corpus

`testdata/adversarial/` keeps one fixture per confirmed hostile or pathological case,
and `internal/engine/adversarial_test.go` registers each one. A case names the fixture,
the expected diagnostic code (or success plus a bounded check, such as the number of
embedded images or pages) and a small `MaxOperations` or another limit. The assertion is
always the work guard's diagnostic or a bounded output, never elapsed time.

To add a fixture:

1. Put the input in `testdata/adversarial/` (HTML, plus a small PNG or font if needed).
2. Add a case to `adversarialCases()` with an id, the fixture name, either `Want` (the
   documented code) or a `Check`, and its `Options` — usually a small limit.
3. Run `go test ./internal/engine -run TestAdversarial`. `TestAdversarialManifest`
   fails and names the file when a listed fixture is missing or an HTML fixture is not
   registered, so the corpus cannot silently shrink.

A fixture whose failure mode is a fatal runtime error, such as a stack overflow, runs
through `TestAdversarialFatalErrorRunsInSubprocess`: the child converts the fixture,
prints its diagnostics and exits normally, and a non-zero exit fails the parent. The
corpus is part of the ordinary unit and race jobs; there is no separate CI step.

The suite bundles `testdata/fonts/DejaVuSans.ttf`, so results are identical on
every machine and in CI. `PDFTEST_FONT=/path/to/font.ttf` overrides it for the
text and conversion tests; the render snapshots always use the bundled font.

### Updating snapshots

```bash
go test ./internal/engine -run TestCorpus -update            # golden texts
go test ./internal/engine -run TestDocumentSnapshots -update # parsed model
go test ./internal/engine -run TestRenderSnapshots -update   # page rasters
go test ./internal/mpdfcompat -update                        # mPDF corpus
go test ./internal/weasycompat -run TestCorpus -update       # WeasyPrint page text
go test ./internal/weasycompat -run TestUpdateOracle -update-oracle  # reference text
```

The WeasyPrint oracle is generated from the reference PDFs with `pdftotext`; the
committed JSON is what the test reads, so a plain `go test ./...` needs no
poppler. Regenerating it with another poppler is a reviewed commit, and
`-update-hashes` recomputes `samples.json` after the vendored files change.

A snapshot update is a reviewed change, not a refresh: the diff says which
behaviour moved. The render snapshots are regenerated only with the poppler
version pinned in CI; moving it and regenerating them is one deliberate commit.

## Formatting and linting

The source is formatted with `gofmt` and checked by `golangci-lint` using
`.golangci.yml`.

```bash
gofmt -w $(git ls-files '*.go')   # format
gofmt -l $(git ls-files '*.go')   # check
golangci-lint run ./...           # lint
```

The pinned version is `golangci-lint` v2.15.0; CI installs exactly that version.
Install the release binary from
[the v2.15.0 release](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0).
Building it from source with
`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`
needs Go 1.26 or newer, which the module's Go minimum already provides.

## Performance

The table below is indicative, not a gate: it was measured on an Apple M2 Pro
with

```bash
go test ./internal/engine -bench 'Convert' -benchtime 10x
```

| Benchmark | Time per run | Allocations |
|---|---|---|
| `BenchmarkConvertOnePage` | ≈ 4 ms | ≈ 7 MB |
| `BenchmarkConvertFiftyPages` | ≈ 91 ms | ≈ 114 MB |

Timing is environment dependent, so the suite asserts no elapsed-time budget and
these numbers are refreshed with a release rather than with every change.
`PDF_PERF=1` runs an opt-in performance report that logs timings and
allocations for the same documents.

## Continuous integration

The test work runs as independent jobs that start together, each reporting its
own check:

| Job | Runs | Tools |
|---|---|---|
| `unit` | `go vet` and the shuffled suite with `PDF_REQUIRE_VALIDATOR=1`, plus the coverage floor | Go, `pdfcpu` |
| `race` | the shuffled suite with the race detector and `PDF_REQUIRE_VALIDATOR=1` | Go, `pdfcpu` |
| `golden` | both snapshot layers explicitly (a skipped layer fails the job) with `PDF_REQUIRE_RENDER=1`, and the failure artifacts | Go, poppler, `pdfcpu` |
| `fuzz` | a short fuzz session per target | Go |
| `lint` | `gofmt` and the pinned `golangci-lint` | Go, `golangci-lint` |

The render tests are required only in `golden`, which runs on the workflow's
`ubuntu-latest` image and installs its poppler; `unit` and `race` skip them, so
they do not pay for a rasterizer. The `gate` job depends on all five and fails when any of
them is not successful, including a skipped one.

Branch protection must require the `gate` check and reject a force push to
`main`; both are repository settings, not files in this repository.

History is append-only. `git push --force` and `git push --force-with-lease` are
forbidden on every branch, including your own feature branch and the branch of an
open pull request: correct a commit with a new commit on top, and undo a pushed
commit with `git revert`. A released tag is never moved or deleted; a fix ships as
a new version. If a branch was rewritten by accident, restore the pushed commits
and land the correction as a new commit.

Remaining inputs stay manual because every bump of them is a decision rather
than a refresh: the poppler the `ubuntu-latest` image ships, which determines the
golden rasters and must be accepted by regenerating them and the renderer record;
`pdfcpu`, whose version must stay buildable by the Go minimum in `go.mod`; the
pinned `golangci-lint` version; and the `go` directive itself.

Action references and Go module requirements are kept current by Dependabot,
configured in `.github/dependabot.yml`: weekly, with a cooldown before a fresh
release is proposed. Action minor and patch bumps travel together, while a
major is alone; module patch updates are grouped, but a module minor or major
arrives alone, because a module below v1 may change its API. No update touches
the `go` directive or adds and removes module requirements — a release under a
new module path is a human change. Nothing merges automatically: a dependency
pull request goes through the same review and the same required checks as any
other change.

## Releasing

1. `CHANGELOG.md` gains the new version and what changed.
2. The suite is green on the exact commit: `go test -shuffle=on ./...`,
   `go test -shuffle=on -race ./...`, both snapshot layers and `pdfcpu`.
3. Tag `vX.Y.Z` and push the tag.
4. Verify consumption from a scratch module:
   `go mod init tmp && go get github.com/vyachin/go-pdf/pdfgen@vX.Y.Z`.

The package is v1 and follows semantic versioning: a minor release may add to
the API but does not rename or remove what exists, and a breaking change takes a
major release. Diagnostic codes, `Options` fields (additive only) and the
deterministic output are the stable surface; `cmd/pdfgen` and `internal/` are
not versioned at all.
