# Contributing

This file is the entry point for working on the engine. The consumer-facing
documentation is the README; nothing here is required reading to use the package.

## Layout

```
  HTML
   |
   v
 html/      scan (positions, limits) + standard tree + model
   |
   v
 css/       tokenizer, parser, selectors, cascade
   |
   v
 profile/   allow-list, validation, diagnostics
   |
   v
 layout/    box tree -> lines -> fragments -> pages
   |   \
   |    text/   shaping, line breaking, metrics
   v
 backend/pdf/     PDF writer, fonts, ToUnicode
 backend/raster/  debug raster for tests
```

`pdfgen` is the only public package; everything under `internal/` is an
implementation detail that Go hides from other modules. Geometry is integer
millipoints (1/1000 pt), so results never depend on floating point.

`cmd/pdfgen` is the reference example and must stay one: it imports `pdfgen`
and the standard library only. CI fails when its direct imports contain an
`internal/` package, because the command's job is to prove the public API is
enough.

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
  compares the character map, the advances, the vertical metrics and every
  outline against it, and `sfnt_subset_test.go` hands it the subsetter's
  rewritten tables. Kerning is not compared — `sfnt` reads a Latin-only subset
  of `GPOS`.
- **Debug raster scaler** (`internal/backend/raster`). Nearest-neighbour
  scaling is deliberate: `testdata/golden/render` is compared pixel by pixel,
  and any interpolation would make every golden depend on the filter.
- **CSS grammar** (`internal/css`). No Go-project module implements the CSS
  tokenizer, parser, selectors or cascade.
- **PDF syntax** (`internal/pdf`). No Go-project module writes the objects, the
  `Identity-H` font, the `ToUnicode` map or the number formatting the output is
  pinned to.
- **Browser-lowering case mapping** (`internal/browser`). Locale-aware case
  mapping behind `text-transform` is a best-effort approximation of a property
  the profile does not support, and every approximation is reported as a
  warning.

## Tests

```bash
go test ./...                                  # unit and integration tests
go test -race ./...                            # the concurrency guarantee
PDF_REQUIRE_VALIDATOR=1 go test ./...          # require pdfcpu, do not skip
go test ./internal/engine -bench .             # benchmarks
```

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

On an Apple M2 Pro: one page ≈ 2.5 ms, a 50-page document ≈ 115 ms with about
102 MB of total allocations.

`go test ./internal/engine -bench .` reports the current numbers; `PDF_PERF=1`
runs an opt-in performance report that logs the timings and allocations. Timing
is environment dependent, so the suite asserts no elapsed-time budget.

## Continuous integration

The test work runs as independent jobs that start together, each reporting its
own check:

| Job | Runs | Tools |
|---|---|---|
| `unit` | `go vet`, the suite with `PDF_REQUIRE_VALIDATOR=1`, the reference command's import check, and a build of the package from a module outside this repository | Go, `pdfcpu` |
| `race` | the suite with the race detector and `PDF_REQUIRE_VALIDATOR=1` | Go, `pdfcpu` |
| `golden` | both snapshot layers explicitly (a skipped layer fails the job) with `PDF_REQUIRE_RENDER=1`, and the failure artifacts | Go, poppler, `pdfcpu` |
| `fuzz` | a short fuzz session per target | Go |
| `lint` | `gofmt` and the pinned `golangci-lint` | Go, `golangci-lint` |

The render tests are required only in `golden`, which owns the pinned
`ubuntu-24.04` image and its poppler; `unit` and `race` skip them, so they do not
pay for a rasterizer. The `gate` job depends on all five and fails when any of
them is not successful, including a skipped one.

Branch protection must require the `gate` check; that is a repository setting,
not a file in this repository.

Remaining inputs stay manual because every bump of them is a decision rather
than a refresh: the `ubuntu-24.04` runner and its poppler, which determine the
golden rasters; `pdfcpu`, whose version must stay buildable by the Go minimum in
`go.mod`; the pinned `golangci-lint` version; and the `go` directive itself.

Action references are kept current by Dependabot, configured in
`.github/dependabot.yml`: one pull request for minor and patch bumps and a
separate one for a major, checked weekly. Nothing merges automatically — a
dependency pull request goes through the same review and the same required
checks as any other change.

## Releasing

1. `CHANGELOG.md` gains the new version and what changed.
2. The suite is green on the exact commit: `go test ./...`,
   `go test -race ./...`, both snapshot layers and `pdfcpu`.
3. Tag `vX.Y.Z` and push the tag.
4. Verify consumption from a scratch module:
   `go mod init tmp && go get github.com/vyachin/go-pdf/pdfgen@vX.Y.Z`.

The package is v1 and follows semantic versioning: a minor release may add to
the API but does not rename or remove what exists, and a breaking change takes a
major release. Diagnostic codes, `Options` fields (additive only) and the
deterministic output are the stable surface; `cmd/pdfgen` and `internal/` are
not versioned at all.
