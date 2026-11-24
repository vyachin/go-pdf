# go-pdf

`github.com/vyachin/go-pdf/pdfgen` converts HTML into a PDF from Go: a pure
library with no cgo, no Chromium and no child process. It is built for printed
business documents — invoices, acts, contracts, reports — where the same
template must reproduce the same file today and in a year.

```go
import "github.com/vyachin/go-pdf/pdfgen"
```

Go 1.26 or newer. The module has no dependencies.

## Install

```bash
go get github.com/vyachin/go-pdf/pdfgen
```

## Use

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
		File:    "invoice.html",      // names the document in diagnostics
		BaseDir: "assets",            // relative images resolve here
		FontDir: "assets/fonts",      // relative @font-face paths resolve here
	})

	var verr *pdfgen.Error
	if errors.As(err, &verr) {
		for _, d := range verr.Diagnostics { // every problem, with a position
			log.Print(d)
		}
		os.Exit(1)
	}
	if err != nil {
		log.Fatal(err) // a cancelled context or an unexpected failure
	}
	for _, d := range res.Diagnostics { // warnings: the PDF exists
		log.Print(d)
	}

	if err := os.WriteFile("invoice.pdf", res.PDF, 0o644); err != nil {
		log.Fatal(err)
	}
}
```

`Validate` runs the same pipeline and returns the same diagnostics without
producing a PDF, which is what a CI check needs. The full reference is the
package documentation:

```bash
go doc github.com/vyachin/go-pdf/pdfgen
```

## Guarantees

- **Deterministic.** The same document and the same options produce a
  byte-identical PDF. `Options.Date` defaults to a fixed constant, not the clock.
- **Nothing is written.** `Convert` and `Validate` create, modify and delete no
  files; they read the assets the document references and return the PDF to the
  caller, who decides where it goes.
- **No panics.** Any input is answered with a result or an error, never with a
  crash, so a hostile template cannot take the host process down.
- **Concurrent.** The package keeps no mutable global state; independent calls
  may run in parallel and agree with sequential ones.
- **Bounded.** `Options.Limits` caps input size, page count, image pixels, font
  bytes, nesting depth and total work. A document over a limit fails with a
  `resource/limit` diagnostic before the expensive allocation.

Every guarantee above is enforced by a test in the package.

## What it supports

A closed profile rather than a browser: blocks, inline text, lists, tables with
fixed layout, `@page` with margin boxes and page counters, `@font-face` with
TrueType fonts, images, and page breaks that respect `orphans`/`widows` and
repeated table headers. The profile has no flexbox, grid, float, absolute
positioning or JavaScript; a browser-oriented template can be lowered to it
first.

- The complete list of elements, properties and limits is in
  [docs/profile.md](docs/profile.md).
- Fonts, subsetting and the copyable text layer: [docs/fonts.md](docs/fonts.md).
- Accepting a browser-oriented template: [docs/browser-import.md](docs/browser-import.md).

## Reference command

`cmd/pdfgen` is a complete example of the package: one file in, one PDF out.

```bash
go install github.com/vyachin/go-pdf/cmd/pdfgen@latest

pdfgen invoice.html              # writes invoice.pdf next to the input
pdfgen -o out.pdf invoice.html   # or where you say
```

Relative images, stylesheets and fonts resolve against the input file's
directory. Exit codes: `0` success, `1` the document cannot be converted, `2` a
usage error.

The command is an example, not a versioned surface: its interface may change in
any release. The source is [cmd/pdfgen/main.go](cmd/pdfgen/main.go), and it
imports nothing but `pdfgen` and the standard library.

## Compatibility

The package is pre-v1 (`v0.x`). The public API is deliberately small, and it is
treated as stable: a minor release may add to it but does not rename or remove
what exists. Diagnostic codes, `Options` fields (additive only) and the
deterministic output are stable; the reference command, the `internal/`
packages and the breadth of the CSS profile are not.

## Contributing

Architecture, the test layers, the golden snapshots, the mPDF corpus, the
formatting and linting workflow, and the release procedure are in
[CONTRIBUTING.md](CONTRIBUTING.md).
