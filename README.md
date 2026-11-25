# go-pdf

`github.com/vyachin/go-pdf/pdfgen` converts HTML into a PDF from Go: a pure
library with no cgo, no Chromium and no child process. It is built for printed
business documents — invoices, acts, contracts, reports — where the same
template must reproduce the same file today and in a year.

```go
import "github.com/vyachin/go-pdf/pdfgen"
```

Go 1.26 or newer. The package needs the standard library and modules the Go
team maintains: `golang.org/x/net` builds the document tree and
`golang.org/x/text` normalizes text. Where the Go project provides a facility
the engine needs, the engine uses it, and a private implementation remains only
where no such module covers the need — [CONTRIBUTING.md](CONTRIBUTING.md) lists
the exceptions and why they stand. Dependencies stay inside that allowlist: the
standard library, or a module the Go team maintains and pins in `go.sum`.

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
TrueType and OpenType/CFF fonts, images, and page breaks that respect
`orphans`/`widows` and repeated table headers. The profile has no flexbox, grid,
float, absolute positioning or JavaScript. There is no lowering pass: a
browser-oriented template must be rewritten within the profile, and the
migration table in [CHANGELOG.md](CHANGELOG.md) lists the replacements for the
removed compatibility layer.

- The complete list of elements, properties and limits is in
  [docs/profile.md](docs/profile.md).
- Fonts, subsetting and the copyable text layer: [docs/fonts.md](docs/fonts.md).

## Input encodings

A template does not have to be UTF-8. The encoding is detected the way the HTML
standard detects it: a byte order mark first, then a `<meta charset>` or
`<meta http-equiv="Content-Type">` declaration in the first 1024 bytes, and
otherwise UTF-8. A UTF-8 byte order mark is removed before parsing, so it never
reaches the page.

The accepted set is closed: **UTF-8, windows-1251, KOI8-R and IBM866**. Anything
else — UTF-16, shift_jis, windows-1252 — fails with `html/encoding`, a message
that names the encoding and lists the supported ones, instead of rendering
replacement characters and then complaining about a missing glyph. A document
that is not valid UTF-8 and declares nothing fails the same way, at its first
invalid byte.

`Options.Charset` names the encoding explicitly for a file exported by a tool
that forgot its declaration, and wins over both the byte order mark and the
declaration:

```go
res, err := pdfgen.Convert(ctx, src, pdfgen.Options{Charset: "windows-1251"})
```

A value outside the set is `options/charset`, reported before any asset is read.
The reference command takes the same setting as `-charset`.

A linked stylesheet is decoded with the document's encoding. A `@charset` rule
that names the same encoding is accepted and ignored; one that names another
fails with `css/link-encoding`. Every diagnostic still points at the original
file: a position inside a decoded document or stylesheet is translated back to
its line and column, and a column counts characters, not bytes, so a diagnostic
after a Cyrillic word names the column the author sees in an editor in a UTF-8
and in a legacy document alike.

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

The package is v2.0.0 and follows semantic versioning: within 2.x a release may
add to the API but does not rename or remove what exists, and a breaking change
takes a major release. The v1 to v2 migration — the removed
`Options.ImportBrowser` field and the stricter validation — is documented in
[CHANGELOG.md](CHANGELOG.md). Diagnostic codes, `Options` fields (additive only) and
the deterministic output are the stable surface; the reference command, the
`internal/` packages and the breadth of the CSS profile are not part of the
promise. Every release is recorded in [CHANGELOG.md](CHANGELOG.md).

## Contributing

Architecture, the test layers, the golden snapshots, the mPDF corpus, the
formatting and linting workflow, and the release procedure are in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

BSD Zero Clause License (0BSD): there is no condition to satisfy and no
attribution is required. See [LICENSE](LICENSE).
