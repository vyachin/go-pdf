# Design

## Context

The project starts from an empty repository: there is no Go module, code or dependencies yet. The only contract in force is `proposal.md` and the `css-profile`, `text-shaping`, `document-layout`, `pagination`, `pdf-output` specs.

Constraints that shape the design:

- one static binary, `CGO_ENABLED=0`, no child processes;
- runtime dependencies: the Go standard library only;
- one engineer at full load, ~60–80 person-weeks;
- target documents — invoices, acts, contracts: controlled templates, Russian text, tables over several pages, page headers and footers, occasionally a table of contents.

## Goals / Non-Goals

**Goals:**

- A deterministic pipeline: the same input gives a byte-identical PDF.
- Pagination as a primary entity, not an add-on over continuous layout.
- One source of truth for the CSS profile, from which the parser, cascade, linter and documentation are generated.
- Diagnostics with source coordinates at every stage, usable in CI.
- Testability without external rasterizers: our own test raster backend.
- The ability to replace any layer without touching the others.

**Non-Goals:**

- Compatibility with arbitrary external HTML.
- Support for `flex`, `grid`, `float`, absolute positioning.
- JavaScript, web fonts by URL, bidi.
- Incremental or streaming PDF delivery (see decision 3).
- PDF/A and encryption in v1.

## Decisions

### 1. Six layers with one-way dependencies

```
  []byte HTML
      |
      v
  html/     tokenizer + tree              (depends on nothing)
      |
      v
  css/      tokenizer, parser, selectors, cascade
      |
      v
  profile/  allow-list, validation, diagnostics
      |
      v
  layout/   box tree -> fragments + pagination
      |            \
      |             v
      |          text/     shaping, line breaking (called from layout)
      v
  render/   painting into an abstract backend
      |
      +--> pdf/    writer, fonts, ToUnicode, images
      +--> raster/ test backend for regressions
```

Dependencies point downwards only. `pdf/` knows nothing about `html/`, `layout/` knows nothing about `pdf/`. Each layer is tested in isolation.

*Alternative*: a monolith without boundaries — faster at the start, but `layout` and `pdf` are then tested only together and any font change breaks the layout tests.

### 2. The internal unit is the integer millipoint

All geometric values are stored as `int32` in 1/1000 pt (1 pt = 1/72 inch). Conversion from CSS happens once, when values are computed.

*Why*: accumulation of float error in long tables produces borders that "jitter" by 1–2 thousandths of a point, which breaks byte determinism and snapshots. An integer in 1/1000 pt gives a resolution of 0.00035 mm — two orders of magnitude finer than printing needs, and exactly representable in PDF (three decimal places).

*Alternative*: `float64` with rounding at write time — simpler, but different architectures and operation orders give different results, and tests become fragile.

### 3. Layout with fragmentation, pagination as the driver

`layout` does not build one endless page. It lays out a container into a **fragmentainer** of bounded height and returns the remaining content together with the resume point.

```go
// Pseudo-signature: lay the children of box out into the frag area,
// while they fit; return the unused remainder.
func placeChildren(ctx *ctx, box *Box, frag Fragmentainer) (Fragment, *Box, bool)
```

The page driver calls layout page after page:

```
done := false
for !done {
    page := newPage(pageNumber)
    _, rest, done := placeChildren(ctx, root, fragmentainerOf(page))
    layoutMarginBoxes(page)   // counter(page), counter(pages) — after layout
    pages = append(pages, page)
    pageNumber++
    root = rest
}
```

*Why*: a page break cannot be "fixed up" after layout — it fundamentally changes the layout (the seam runs through a block, changing its inner padding and the repetition of the table header). Resumable layout is the only model that expresses this honestly, and it also provides the repeat passes for the table of contents.

*Alternative*: lay everything out into one long page and cut it by height — cheap, but breaks `break-inside: avoid`, repeated `thead`, `widows`, borders at the seam and page headers and footers flowing around the content. Rejected as a source of irreducible defects.

*Alternative*: stream the PDF as layout proceeds — excludes the second pass, and therefore the table of contents and bookmarks. Rejected.

### 4. Two passes for `target-counter`, converging by page count

A table of contents needs page numbers that themselves depend on the size of the table of contents. The solution: layout is repeated until the "anchor → page" mapping stabilises.

```
pass 1: layout -> map[anchor]page
pass 2: substitute map into target-counter -> layout -> map2
        if map2 == map -> done
        otherwise repeat, at most 4 passes -> diagnostic
```

*Why four*: on real documents a table of contents occupies one page and converges in 2 passes; the upper bound protects against cycles where "the table of contents grows from its own number".

### 5. The profile as a data table, not as code

The list of supported properties is a table: name, value type, whether it is inherited, initial value, applicable elements. From it are generated: value parsing in `css/`, checking in `profile/`, the default stylesheet, the `--print-profile` help.

*Why*: three places where the profile can diverge (parser, linter, documentation) are the most common source of "the template passed validation but rendered wrong".

### 6. Our own PDF writer that preserves the original glyph numbers

The PDF is written directly: objects, cross-reference table (classic, not streamed), streams with `FlateDecode` (stdlib `compress/flate`), fonts as Type0/`Identity-H` + `CIDFontType2`/`FontFile2`, `ToUnicode` for every font.

Subsetting does not renumber glyphs: unused contours are zeroed out, numbers are preserved, `CIDToGIDMap` stays `/Identity`.

*Why*: renumbering requires rebuilding `cmap`, `hmtx`, `loca`, `glyf` and accounting for composite glyphs — the most error-prone part of the writer. The price of declining it is a few kilobytes of empty contours per font.

*Alternative*: `go-pdf/fpdf` — excluded by the zero-dependency requirement and by the fact that its API gives no control over `ToUnicode` and object order.

### 7. TrueType parsing for TrueType outlines only

`head`, `hhea`, `maxp`, `hmtx`, `cmap` (formats 4 and 12), `loca`, `glyf`, `name`, `OS/2`, `GPOS` (PairPos format 1 and 2) are read. CFF outlines, variable fonts and WOFF/WOFF2 are not supported and are rejected at validation.

*Why*: CFF is a separate charstring interpreter with its own subsetting, comparable in scope to the whole rest of the font layer. PT Serif, PT Sans, Liberation and DejaVu are enough for Russian documents — all of them TrueType.

### 8. Simplified shaping: cmap + pair kerning

Text is mapped 1:1 through `cmap`, then `GPOS` pairs are applied. Diacritic composition is solved by normalising the input text to NFC rather than by mark positioning; if the composite glyph is absent, it is an error naming the character.

*Why*: full shaping is needed for Arabic, Devanagari and Latin ligatures. For Russian text in business documents it gives no visible gain but adds months.

### 9. Painting through an abstract backend

`render/` does not know where it draws. The interface is minimal: fill a rectangle, fill a path, draw glyphs, draw an image, set a clip.

- `pdf/` implements the backend on top of our own writer;
- `raster/` implements the same interface into `image.RGBA`: rectangles and paths are filled by our own scanline rasterizer with supersampling, glyphs by `glyf` contours with the nonzero fill rule, images are copied as is.

*Why*: visual regression becomes possible without external rasterizers and cgo. A test renders a fixture to PNG and compares it with the reference image with a tolerance for antialiasing.

*Price*: our own contour rasterizer — 1–2 weeks. That is cheaper than depending on pdfium, and it is reused in the debug `--render-png`.

### 10. Test strategy: layout snapshots, not reference PDFs

Three levels:

1. **Structural snapshots** of `layout`: a text tree of fragments with coordinates, pages and line numbers. Readable in a diff, stable, independent of the font version.
2. **Output validation**: every generated PDF is checked by `pdfcpu validate` and veraPDF in tests (external tools, not runtime).
3. **Text extraction**: a check that Russian text is copied and found by search — via `ToUnicode`, without external libraries.

*Alternative*: reference PDFs in the repository — rejected: any font update or change of object order gives a diff over the whole file and the test stops meaning anything.

### 11. Determinism as a writer invariant

`/ID` — SHA-256 of the document content; `/CreationDate` — from an explicit parameter, a fixed constant by default. Objects are written in creation order, without iterating over `map`. From this, byte-for-byte reproducibility follows for free.

### 12. Package boundaries

```
cmd/pdfgen/            CLI: convert, validate, --render-png
internal/html/         tokenizer, tree builder
internal/css/          tokenizer, parser, selector, cascade
internal/profile/      property table, validation
internal/font/         ttf, cmap, subset, gpos
internal/text/         shape, linebreak
internal/layout/       boxes, block, inline, table, fragment, paginate
internal/render/       painting
internal/backend/pdf/  PDF backend
internal/backend/raster/ test backend
internal/diag/         diagnostics with positions
```

### 13. Generated content: only `a::after`, two-pass `target-counter`

A table of contents needs page numbers, and in CSS they are expressed only through the `content` of a pseudo-element. The profile stays closed: exactly one pseudo-element is allowed — `a::after` — and exactly one property — `content` with strings, `counter(page)`, `counter(pages)` and `target-counter(attr(href), page)`.

Reference resolution is a convergence loop in the engine, not in layout:

```
targets := {}
repeat at most 4 times:
    pages := layout(doc, targets)
    next := { id -> page for every href of an <a> }
    if next == targets: stop
    targets = next
```

*Why a limit of 4*: numbers change only when a page boundary is crossed, and a digit that grew by one character almost never shifts the break. Four passes cover "the table of contents shifted the sections" with margin, and the limit protects against a cycle.

*Why not a general `content`*: generating content for arbitrary elements is another layer (counters, strings, quotes) with unclear value for documents. One scenario (the table of contents) is covered by one pseudo-element.

*An unknown target* (`href` to a missing id) gives a diagnostic and the number `0`, and the document is still created: the table of contents must not fail the whole invoice.

## Risks / Trade-offs

- **Pagination edge cases** — the most expensive part: `rowspan` across the seam, nested tables, paragraphs that float in height. → Constrain the profile: `rowspan` does not cross a page (validation error), nested tables are forbidden in v1, run a corpus of real documents once a week.
- **Subsetting errors give a broken PDF** that only fails in Acrobat. → `pdfcpu validate` and veraPDF in CI on every fixture, plus fuzzing of the font tables.
- **Bus factor 1** → profile and specs as the contract, layout snapshots instead of verbal agreements, decisions documented in this file.
- **The temptation to widen the profile** ("let's add flex") will eat the schedule. → A profile change goes through a spec change, not through a commit to the table.
- **Our own rasterizer** may give a picture that does not match the PDF output and create false confidence in tests. → The raster counts as auxiliary; the truth is layout snapshots and PDF validation.
- **Performance on tables of hundreds of rows** may turn out worse than expected. → Target budgets: page < 20 ms, 50 pages < 300 ms, peak memory < 100 MB; benchmarks on M2.
- **Declining to reuse HTML/CSS tokenizers** costs ~8 weeks. → The decision is deliberate (zero dependencies), and the layer boundaries allow replacing the implementation later without changing the rest.

## Migration Plan

There is no migration: the repository is empty, there are no consumers. Delivery goes by milestones, each milestone a working slice, not "half a layer":

| Milestone | Content | Definition of done |
|---|---|---|
| M0 | HTML tokenizer, minimal CSS, TTF + `cmap`, PDF writer, one line of text | Russian text is extracted from the PDF, the file passes the validator |
| M1 | blocks, paragraphs, inline, lists, fonts by family | single-page documents lay out according to snapshots |
| M2 | tables (fixed), fragmentation, breaks, `thead` | an invoice over 3 pages with a repeated header |
| M3 | `@page`, margins, margin boxes, `counter(page)`/`counter(pages)` | headers and footers on every page, numbers correct |
| M4 | `target-counter`, table of contents, bookmarks, metadata, determinism | byte-identical runs, the table of contents converges |

Rollback: until M3 the engine is not connected to the generation of production documents; the CLI and the library are published as experimental so as not to create backward compatibility ahead of time.

## Open Questions

- Is PDF/A-2b needed in the long run (it affects the requirements for fonts, metadata and colour profiles, but not the architecture).
- Is merging several HTML files into one document needed (cover + appendices) — currently out of scope, implemented as a composition at the page driver level.
- Whether to ship fonts together with the binary via `embed` (plus ~1.5 MB per family) or require them in the template.
- PNG alpha channel support: in v1 transparency is rejected by validation; `SMask` is a candidate for v2.
