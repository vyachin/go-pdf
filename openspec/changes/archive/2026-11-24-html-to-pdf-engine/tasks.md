# Tasks

## 1. Module skeleton

- [x] 1.1 Initialize the Go module and the package directories from design.md — check: `go build ./...` and `go vet ./...` pass on the empty skeleton
- [x] 1.2 Implement the `internal/diag` package: a `Diagnostic` type with file, line, column and element path, deterministic sorting and formatting — check: a unit test on the composition and order of diagnostics
- [x] 1.3 Create the `cmd/pdfgen` CLI with the `convert` and `validate` subcommands and exit codes 0/1/2 — check: a test checks the exit codes for success and failure

## 2. HTML

- [x] 2.1 Implement the HTML tokenizer: tags, attributes, comments, entities, raw text — check: the case table passes, 5 minutes of fuzzing without panics
- [x] 2.2 Implement tree construction: implicit `html`/`head`/`body`, implicit closing of `p`/`li`/`td`, foster-parenting for tables — check: tree snapshots for 10 fixtures match
- [x] 2.3 Store the file, line and column of the start and end of every node — check: a test compares coordinates for a multi-line template
- [x] 2.4 Define the internal `html.Parse` API with package documentation — check: the example in `_test.go` compiles and gives the expected tree

## 3. CSS

- [x] 3.1 Implement the CSS tokenizer: identifiers, numbers, dimensions, strings, `url`, blocks, `!important` — check: the token table passes
- [x] 3.2 Implement the rule parser: style rules, `@page`, `@font-face`, `@media print`, declarations, shorthand properties — check: a test expands `margin` and `border` into longhand properties
- [x] 3.3 Implement selectors: element, class, id, compound, descendant, child, selector list, specificity — check: the table of "selector → specificity" pairs matches
- [x] 3.4 Implement the cascade and inheritance: origin, specificity, order, inline style, initial values — check: the tests for the priorities from the `css-profile` spec pass
- [x] 3.5 Implement values and units with conversion to millipoints — check: a test on `100mm + 1.5em` at `12pt` gives exact values without loss

## 4. Profile

- [x] 4.1 Define the profile property table (value type, inheritance, initial value) and generate the default stylesheet from it — check: a test walks the table and confirms that a parser and a validator exist for every property
- [x] 4.2 Implement the template validator: all errors in one pass, source coordinates, deterministic order — check: fixtures with 1, 3 and 0 errors give the expected reports
- [x] 4.3 Wire the linter into the `validate` CLI — check: an integration test on a non-zero exit code without creating a PDF
- [x] 4.4 Implement `--print-profile`, generated from the table — check: a test compares the output with the property table

## 5. Fonts

- [x] 5.1 Implement TrueType table parsing: `head`, `hhea`, `maxp`, `hmtx`, `name`, `OS/2`, `loca`, `glyf` — check: reading two different fonts gives the expected metrics
- [x] 5.2 Implement `cmap` formats 4 and 12 and glyph lookup by code point — check: the test finds Cyrillic glyphs and "ё", a missing character gives an error
- [x] 5.3 Implement subsetting that preserves glyph numbers, including composite outlines — check: the subset passes the validator and opens in the test backend
- [x] 5.4 Implement reading `GPOS` kerning pairs (PairPos 1 and 2) — check: a test on a font with kerning gives distances different from `hmtx`
- [x] 5.5 Implement `@font-face` for local files with family and style resolution — check: rejection for CFF, WOFF and URL is covered by tests
- [x] 5.6 Make and document the decision on shipping fonts (`embed` or external files) — check: `docs/fonts.md` contains the decision, the rationale and an example

## 6. Text

- [x] 6.1 Implement NFC normalisation for Cyrillic and Latin — check: a decomposed "й" gives one glyph and one character on extraction
- [x] 6.2 Implement shaping: string → sequence of glyphs with advances and kerning — check: a test on "Счёт № 42" gives the expected widths
- [x] 6.3 Implement line breaking that accounts for spaces, hyphens and the non-breaking space — check: a case table, including "г.\u00A0Москва" on one line
- [x] 6.4 Implement justification with an unstretched last line — check: a test on a paragraph of three lines
- [x] 6.5 Compute line metrics: ascent, descent, `line-height` — check: a test compares baselines at `line-height: 18pt` and `font-size: 12pt`

## 7. PDF writer and milestone M0

- [x] 7.1 Implement the PDF object model: dictionaries, arrays, streams, strings, serialization, xref, trailer — check: a single-page file passes `pdfcpu validate`
- [x] 7.2 Implement the content stream: text, rectangles, paths, clip, images — check: a test decodes the stream and finds the expected operators
- [x] 7.3 Implement font writing: Type0/`Identity-H`, width array, `FontFile2`, `ToUnicode` — check: the extracted text matches the original, including Cyrillic
- [x] 7.4 Implement images: JPEG without recompression, PNG through `FlateDecode`, rejection on an alpha channel — check: a test matches the JPEG bytes in the PDF against the original
- [x] 7.5 Assemble the end-to-end M0 slice: the `convert` CLI for a line of Russian text — check: the PDF is valid, the text is extracted and found by search
- [x] 7.6 Fill `Title`/`Author`/`Description` metadata from the document — check: a test reads the Info dictionary

## 8. Layout

- [x] 8.1 Implement the box tree with box types and anonymous blocks — check: snapshots for mixed content match
- [x] 8.2 Implement the block model: `content-box`, collapsing of vertical margins, borders and padding — check: geometry snapshots for 8 cases
- [x] 8.3 Implement inline layout: line boxes, baseline, `vertical-align`, images in a line — check: line height snapshots
- [x] 8.4 Implement whitespace collapsing for `normal` and `pre` — check: snapshots on a multi-line source
- [x] 8.5 Implement lists: markers, `decimal`, `start`, nesting — check: snapshots of numbering and indents
- [x] 8.6 Wire painting to the PDF backend and assemble milestone M1: single-page documents — check: `--render-png` and the PDF match visually for three fixtures

## 9. Raster backend

- [x] 9.1 Implement a scanline path rasterizer with supersampling — check: a test on a circle and a rectangle with a known area
- [x] 9.2 Implement glyph rasterization by `glyf` contours with the nonzero rule — check: a test compares "С" and "о" with reference masks
- [x] 9.3 Implement an image comparison helper with a tolerance and the `--render-png` output — check: a test catches a 1-pixel shift and tolerates antialiasing

## 10. Tables

- [x] 10.1 Implement the table structure and fixed layout by the first row — check: column width snapshots
- [x] 10.2 Implement `colspan` and `rowspan` within one page — check: snapshots for three combinations
- [x] 10.3 Implement `border-collapse: collapse` and `separate` — check: visual regression for a 3×3 table
- [x] 10.4 Wire tables into the PDF output and assemble the M2 slice for one page — check: an invoice with a table renders and passes the validator

## 11. Pagination

- [x] 11.1 Implement block fragmentation: layout into a bounded height and a resume point — check: a unit test on an artificial tree of 20 blocks
- [x] 11.2 Implement `break-before`/`break-after`/`break-inside` and the priority of `page` over `avoid` — check: the case table from the spec
- [x] 11.3 Implement `orphans` and `widows` with default values 2/2 — check: tests on a single first and last line
- [x] 11.4 Implement `thead` repetition and `tfoot` output only on the last page — check: a table over three pages with a snapshot
- [x] 11.5 Handle an element taller than the page: overflow, diagnostics, no looping — check: a test with a timeout on a line taller than the page
- [x] 11.6 Assemble the M2 slice: an invoice over three pages with a repeated header — check: the PDF and the raster match the reference image

## 12. Pages and headers/footers

- [x] 12.1 Implement `@page` with `size` and `margin`, CSS taking priority over the run parameter — check: tests on A4/A5 and explicit sizes
- [x] 12.2 Implement the six margin boxes with content from strings — check: a test on all positions
- [x] 12.3 Implement `counter(page)` and `counter(pages)` — check: a test on a three-page document with "из 3" on every page
- [x] 12.4 Assemble the M3 slice: an invoice with a page footer — check: the footer is present on the first and the last page

## 13. Table of contents and bookmarks

- [x] 13.1 Collect anchors and the pages they landed on — check: a test on a document with five anchors
- [x] 13.2 Implement `target-counter(attr(href), page)` with repeat passes and a limit of 4 — check: tests on convergence, on a shifted break and on unreached convergence
- [x] 13.3 Implement the bookmark tree from `h1`–`h3` with nesting and page binding — check: a test on the "h1 + two h2" structure
- [x] 13.4 Assemble the M4 slice: a document with a table of contents and bookmarks — check: the numbers in the table of contents match the actual pages

## 14. Determinism and quality

- [x] 14.1 Make the output deterministic: `/ID` from the content hash, `/CreationDate` from a parameter, fixed object order — check: two runs give the same `sha256`
- [x] 14.2 Add benchmarks: page < 20 ms, a 50-page document < 300 ms, peak memory < 100 MB — check: `go test -bench` stays within the budgets on M2
- [x] 14.3 Add fuzz tests for the HTML/CSS tokenizers, TTF parsing and the PDF writer — check: 10 minutes of fuzzing without crashes
- [x] 14.4 Assemble a corpus of three real documents (invoice, act, contract) with snapshots and validation in CI — check: `go test ./...` is green, the validators find no errors
- [x] 14.5 Write `README.md` and `docs/profile.md` with the profile, the CLI and the limitations — check: all commands from the README run as written

## Workflow follow-up

- Archive the change after review and the document corpus check pass.
- Check the archive result: the specs are moved to `openspec/specs/`, the change is moved to the archive.
