# Contributing

This file is the entry point for working on the engine. The consumer-facing
documentation is the README; nothing here is required reading to use the package.

## Layout

```
  HTML
   |
   v
 html/      tokenizer + tree
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
  layout; `testdata/fixtures/` adds one small document per profile feature.
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
- Fuzz targets cover the HTML and CSS tokenizers, TrueType parsing, the PDF
  writer and the public `Convert` boundary (arbitrary bytes never panic).

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
enables the elapsed-time budget test. Those budgets are environment dependent
and run only on request, never as part of the ordinary suite.

## Continuous integration

The `test` job runs `go vet`, the full suite with `PDF_REQUIRE_VALIDATOR=1` and
`PDF_REQUIRE_RENDER=1`, the race detector, both snapshot layers explicitly (a
skipped layer fails the job), a short fuzz session per target, the reference
command's import check, and a build of the package from a module outside this
repository. The `lint` job runs `gofmt` and `golangci-lint`.

The `test` job must be a required check in branch protection; that is a
repository setting, not a file in this repository.

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
