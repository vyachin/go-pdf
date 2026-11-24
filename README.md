# go-pdf

An HTML → PDF converter written in pure Go for printed business documents:
invoices, acts, contracts and reports.

One static binary. No cgo. No Chromium, WeasyPrint or child processes. The only
runtime dependency is the standard library.

```bash
pdfgen convert -o invoice.pdf invoice.html
pdfgen validate invoice.html
```

Flags come before the input path: the standard Go flag parser stops at the
first positional argument.

## Why a custom engine

Off-the-shelf options did not fit: Chromium and WeasyPrint are external
processes with a heavy runtime, and the pure-Go port of WeasyPrint (alpha, one
maintainer) is too risky for documents that must reproduce for years.

Instead of CSS breadth, the engine offers three things those engines cannot:

- **Determinism.** Same input → byte-identical PDF. `/CreationDate` is fixed and
  `/ID` is a content hash.
- **Pagination as a first-class concept.** `break-*`, `orphans`/`widows`,
  repeated `thead`, `@page` with margin boxes and counters are part of layout,
  not an add-on.
- **A checkable profile.** An unknown property is an error with a position in the
  template instead of a silent no-op.

## Features

- HTML tokenizer and tree with implicit tags and table foster-parenting.
- CSS: cascade, specificity, inheritance, shorthands, `@page` (including
  `landscape`/`portrait`), `@font-face`, `@media print`, margin boxes, local
  `<link rel="stylesheet">`.
- Layout: blocks, inline, lists, fixed table layout, `colspan`/`rowspan`,
  collapsed borders.
- Pagination: `break-before/after/inside`, `orphans`, `widows`, repeated table
  headers, overflow with diagnostics.
- Fonts: TrueType, `cmap` 4/12, `GPOS` kerning, subsetting, `Identity-H` +
  `ToUnicode` (Russian text can be copied and searched).
- Images: PNG and JPEG (JPEG without recompression).
- Generated content: `a::after` with `target-counter(attr(href), page)` — a table
  of contents with page numbers in two layout passes.
- PDF: metadata, bookmarks from `h1`–`h3`, `FlateDecode`, deterministic output,
  atomic file writing.
- Debug raster: `--render-png` and `-print-profile`.

## CLI

```bash
pdfgen convert [flags] <input.html>
pdfgen validate [flags] <input.html>
pdfgen help
```

| `convert` flag | Meaning |
|---|---|
| `-o` | output PDF path (default `<input>.pdf`) |
| `-page` | fallback page size: `A4`, `A5`, `Letter`, `210x297mm` |
| `-margin` | fallback margins: `20mm`, `25mm 20mm` |
| `-date` | creation date in RFC 3339 (fixed by default) |
| `-render-png` | also rasterize the first page to `<output>.png` |
| `-strict` | treat warnings as errors (for CI) |
| `-font-dir` | directory for relative `@font-face` paths |
| `-base-dir` | directory for relative image paths |

Exit codes: `0` success, `1` validation or rendering error, `2` usage error.

```bash
# Show the profile the engine understands
pdfgen validate -print-profile

# Validate without rendering (useful in CI)
pdfgen validate invoice.html && pdfgen convert -o invoice.pdf invoice.html

# Strict mode: warnings (for example about an ignored <script>) fail the build
pdfgen validate -strict invoice.html
```

## Use as a library

`github.com/vyachin/go-pdf/pdfgen` is the public API. The CLI drives the same
engine, so its flags and `pdfgen.Options` describe the same knobs, while
`internal/` stays an implementation detail that Go hides from other modules.

```bash
go get github.com/vyachin/go-pdf/pdfgen
```

```go
package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/vyachin/go-pdf/pdfgen"
)

func main() {
	src, err := os.ReadFile("invoice.html")
	if err != nil {
		log.Fatal(err)
	}
	res, err := pdfgen.Convert(context.Background(), src, pdfgen.Options{
		File:     "invoice.html", // appears in diagnostics
		PageSize: "A4",           // fallback when @page has no size
		Margin:   "20mm",
		BaseDir:  "assets", // relative images stay inside this directory
		FontDir:  "fonts",  // relative @font-face paths
	})
	var verr *pdfgen.Error
	if errors.As(err, &verr) {
		for _, d := range verr.Diagnostics {
			log.Print(d) // invoice.html:7:12: error[css/unsupported-value]: ...
		}
		os.Exit(1)
	}
	if err != nil {
		log.Fatal(err) // cancelled context or an unexpected failure
	}
	for _, d := range res.Diagnostics { // warnings only
		log.Print(d)
	}
	if err := os.WriteFile("invoice.pdf", res.PDF, 0o644); err != nil {
		log.Fatal(err)
	}
}
```

The same call shape works for validation. `Validate` runs the cheap half of the
pipeline — parsing, cascade and profile — and reports the same diagnostics
without paying for layout, which is what a CI check or a "preview" endpoint
needs:

```go
diags, err := pdfgen.Validate(ctx, src, opts)
var verr *pdfgen.Error
switch {
case errors.As(err, &verr):
	log.Fatal(verr) // the template cannot be rendered
case err != nil:
	log.Fatal(err)
}
for _, d := range diags { // warnings only
	log.Print(d)
}
```

- **Nothing touches the disk.** The package only reads the images, stylesheets
  and fonts the template references; writing the PDF (atomic or not) and
  storing it stay the caller's decision.
- **Determinism carries over.** Same template and same options → byte-identical
  PDF. `Options.Date` is a fixed constant by default; set
  `opts.Date = time.Now()` for documents that must carry today's date.
- **One pipeline, two entry points.** `Validate` accepts exactly the templates
  `Convert` renders, because both run the same preparation step.
- **Diagnostics are data.** `Diagnostic` has a stable `Code` (`css/unsupported-value`,
  `resource/limit`, …), a 1-based position and `String()` in the CLI format, so
  a server can return them as JSON instead of parsing log lines.
- **Previews.** `Options.RenderPNG` puts a raster of the first page in
  `res.PNG`.
- **Budgets.** `Options.Limits` bounds input and stylesheet size, page count,
  image pixels and encoded image bytes, font bytes, DOM depth, one table's grid,
  the debug page raster and the total work of a conversion. An over-limit document returns a
  `*pdfgen.Error` with code `resource/limit` before the expensive allocation,
  not an OOM.
- **Cancellation.** Pass a context that can be cancelled: parsing, stylesheet
  collection, font loading, layout between pages, rows and lines, PDF
  serialization and the debug raster all stop when it is done, and `Convert`
  returns the context error instead of a partial PDF.
- **Strict mode.** `Options.Strict` turns warnings into errors, the same way
  `pdfgen validate -strict` does.

The API reference is available through the usual tooling:

```bash
go doc github.com/vyachin/go-pdf/pdfgen
go doc github.com/vyachin/go-pdf/pdfgen.Options
```

## Profile

The full list of elements, properties and limits is in [docs/profile.md](docs/profile.md).
In short: blocks, inline, tables, lists, `@page`, `@font-face`, `counter(page)`,
`counter(pages)`.

Not supported: `flex`, `grid`, `float`, absolute positioning, JS,
`hyphens: auto`, CFF/OTF, web fonts, `table-layout: auto`.

Fonts are documented separately: [docs/fonts.md](docs/fonts.md).

## Architecture

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

Geometry is integer millipoints (1/1000 pt), so results do not depend on
floating point.

## Guarantees

- A broken template produces no PDF: validation runs before rendering.
- A missing glyph is an error naming the character and its position, not a
  blank space.
- Two runs of the same command produce the same file.
- Writing the result is atomic: on error the previous file stays intact.

## Testing

```bash
go test ./...
go test ./internal/engine -run TestCorpus         # document corpus
go test ./internal/engine -run TestCorpus -update # rebuild golden texts
go test ./internal/engine -bench .
go test ./internal/mpdfcompat              # mPDF corpus
go test ./internal/mpdfcompat -update      # rebuild golden texts
```

- Layout snapshots check geometry, not pixels.
- Generated PDFs are validated by the independent `pdfcpu` tool when it is
  available in `PATH` (tests also honour `PDFCPU`); CI requires it through
  `PDF_REQUIRE_VALIDATOR=1`.
- The `testdata/*.html` corpus is an invoice, an act and a contract with real
  layout.
- The mPDF corpus (`internal/mpdfcompat`) contains 149 documents extracted from
  the mPDF tests. It checks that a document inside the profile yields a valid
  deterministic PDF, while a document outside the profile is rejected with
  diagnostics instead of a crash. Details and the limitation map are in
  [docs/mpdf-corpus.md](docs/mpdf-corpus.md).
- Fuzz tests cover the HTML and CSS tokenizers, TrueType parsing and the PDF
  writer.
- Visual regressions use the engine's own raster backend and `raster.Diff`.

The test suite bundles `testdata/fonts/DejaVuSans.ttf`, so results are identical
on every machine and in CI. `PDFTEST_FONT=/path/to/font.ttf` overrides it.

## Performance

On an Apple M2 Pro: one page ≈ 2.5 ms, a 50-page document ≈ 115 ms with about
102 MB of total allocations.

## Status

| Milestone | Content | State |
|---|---|---|
| M0 | Russian text in a valid PDF | done |
| M1 | blocks, inline, lists | done |
| M2 | tables, pagination, repeated headers | done |
| M3 | `@page`, margin boxes, counters | done |
| M4 | `target-counter`, table of contents, bookmarks, metadata | done |
