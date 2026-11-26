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
		// The face for families the document does not declare. Explicit font
		// bytes keep the output identical on every machine.
		FallbackFont: "assets/fonts/body.ttf",
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

### Fonts

A family resolves in one documented order: an `@font-face` family, then
`Options.FallbackFont`, then the ordered `Options.FallbackFonts` list, then the
`font/unavailable` diagnostic. The library reads no installed font on its own,
so the same input and the same font file produce the same PDF on every machine.
`FallbackFont` is one explicit path; `FallbackFonts` is a list whose first
loadable entry becomes the family, with its bold and italic siblings discovered
next to it.

To opt into the host's conventional fonts, pass them explicitly:

```go
res, err := pdfgen.Convert(ctx, src, pdfgen.Options{
	FallbackFonts: pdfgen.SystemFallbackFonts(), // Arial, DejaVu, Liberation, ...
})
```

That is the one way host-dependent output enters the library; the zero
`Options` value never consults the list. `SystemFallbackFonts` returns a fresh
copy, so a caller may modify it. See [fonts and resources](docs/fonts.md) for the
full order and the failure diagnostic.

### Asset roots and confinement

A relative image or stylesheet reference resolves against `Options.BaseDir`; a
relative `@font-face` source resolves against `Options.FontDir`, then `BaseDir`.
When those roots are empty, the directory of `Options.File` is the root, and when
`File` is empty as well, the process working directory is. `File` stays the name
a diagnostic reports; only its directory selects the default root, so
`File: "<stdin>"` resolves relative assets in the working directory.

Every relative reference is confined to its root: a `..` path or a symlink that
leaves it fails with the resource's load diagnostic (`img/load`,
`css/link-path` or `font/unavailable`) and no PDF. An absolute reference is
accepted only inside a configured root; with no configured root it is rejected,
because the document directory is not a boundary the caller named. A caller that
must assemble a template and its assets under absolute paths opts in per
conversion:

```go
res, err := pdfgen.Convert(ctx, src, pdfgen.Options{
	File:                "invoice.html",
	AllowAbsoluteAssets: true, // trusted input only
})
```

`FallbackFont` and `FallbackFonts` are caller-selected paths and are always read
as written, whatever the option says. `Options.Source` keeps its own namespace:
paths are relative there, so an absolute reference is rejected even with the
option set — see [fonts and resources](docs/fonts.md).

### Assets in an `fs.FS`

`Options.Source` reads every asset through a caller-supplied `fs.FS` instead of
the host filesystem, so templates and their resources can ship inside the binary
without extracting anything:

```go
//go:embed assets
var assets embed.FS

res, err := pdfgen.Convert(ctx, src, pdfgen.Options{
	File:    "invoice.html", // the diagnostic name, not a source path
	Source:  assets,
	BaseDir: "assets",       // image and stylesheet directory inside the source
	FontDir: "assets/fonts", // @font-face directory inside the source
})
```

`BaseDir` and `FontDir` are paths inside the source and default to its root;
`FallbackFont` and `FallbackFonts` are paths inside it too, read from the source
root. The source is authoritative: no host file and no installed host font is
consulted, so ship the font in the source and declare it with `@font-face` or set
`FallbackFont`/`FallbackFonts`. A source path is relative and must stay inside
the source; an absolute or escaping reference fails with the same `img/load`,
`css/link-path` or `font/unavailable` diagnostic as a filesystem escape. Leaving
`Source` nil keeps the filesystem behaviour unchanged.

### Streaming a large document

`ConvertTo` writes the PDF to an `io.Writer` as it is serialised, so a server
does not hold the complete document in memory:

```go
out, err := os.Create("invoice.pdf")
if err != nil {
	log.Fatal(err)
}
defer out.Close()

res, err := pdfgen.ConvertTo(context.Background(), out, src, pdfgen.Options{
	File: "invoice.html",
})
if err != nil {
	var verr *pdfgen.Error
	if errors.As(err, &verr) {
		for _, d := range verr.Diagnostics { // includes pdf/write
			log.Print(d)
		}
	} else {
		log.Print(err) // a cancelled context
	}
	// Discard the output. A validation error wrote nothing; a write error or a
	// cancelled context may have left a prefix.
	return
}
for _, d := range res.Diagnostics { // warnings: the PDF was written
	log.Print(d)
}
```

`ConvertTo` reports the same diagnostics as `Convert`, produces the same bytes
and leaves `Result.PDF` nil. Serialisation holds at most one page's content
stream, so the memory needed to write the PDF does not grow with the number of
pages.

## Guarantees

- **Deterministic.** The same engine build, the same HTML bytes, the same
  options and the same resolved asset bytes — linked stylesheets, images and the
  selected font bytes — produce a byte-identical PDF. `Options.Date` defaults to
  a fixed constant, not the clock, and the document identifier is a hash of the
  content, so two runs agree at any wall-clock time. Every input has to be held
  still: an asset edited in place is a changed input, and the engine selects no
  installed font on its own, so the bytes come from an `@font-face` declaration
  or an explicit fallback and the same template ships the same font. Keep the
  resources immutable — see [fonts and resources](docs/fonts.md).
- **Nothing is written.** `Convert`, `ConvertTo` and `Validate` create, modify
  and delete no files; they read the assets the document references — from the
  host filesystem or from the caller-supplied `Options.Source` — and return the
  PDF to the caller, who decides where it goes. On the host filesystem a read
  stays inside the conversion's asset root, and an escaping relative reference
  or an absolute one outside a configured root is rejected with a diagnostic.
  `ConvertTo` writes only to the writer it is given.
- **Streaming.** `ConvertTo` writes the PDF to an `io.Writer` as it is
  serialised and holds at most one page's content stream, so the memory needed
  to serialise the document does not grow with the number of pages. It produces
  the same bytes as `Convert` and reports the same diagnostics, with
  `Result.PDF` nil. A write error or a cancelled context can leave a prefix in
  the writer, which the caller discards.
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
directory. The command passes `pdfgen.SystemFallbackFonts()` so a one-file
conversion still works on a developer machine; that is an example opt-in, not
the library default: the `pdfgen` package reads no host font unless the caller
names one. Exit codes: `0` success, `1` the document cannot be converted, `2` a
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
