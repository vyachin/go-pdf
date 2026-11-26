# Changelog

All notable changes to this project are documented in this file. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Text source positions.** A text node written with a character reference
  (`&amp;`), one whose first newline `pre`/`listing`/`textarea` drops, or one
  around a NUL now carries its own token's range, so a diagnostic names the text
  instead of its parent element. Equal out-of-source-order texts keep distinct
  positions. Matching a text node to its token is indexed and charged to the work
  budget, so a document of references no longer does uncharged quadratic work.
- **Malformed GPOS feature list.** A `FeatureList` offset outside a font's GPOS
  table now disables that font's kerning instead of reading outside the table or
  panicking; the font stays usable.
- **Malformed CSS escapes.** A style attribute or stylesheet that ends with a
  lone `\` now reports `css/syntax` instead of hanging, and declaration parsing is
  charged to the work budget like stylesheet parsing.

## [2.0.0] - 2026-11-25

### Removed

- **Browser import.** The opt-in compatibility layer and its
  `Options.ImportBrowser` field are removed from the `pdfgen` package and the
  reference command. A template that relied on lowering (flex columns, absolute
  cover images, border-box sizing, `rem`/`vw` units, custom properties, screen
  media rules, cosmetic-property removal) is now rejected by the closed profile
  with ordinary diagnostics and no PDF. Callers must remove the field from their
  option literals and rewrite the template within the documented profile; the
  browser-oriented certificate remains in the test corpus as a rejection fixture.

### Migration from 1.x

This is a major release: it removes one exported field and tightens validation.
A caller migrating from v1.0.0:

1. Deletes `Options.ImportBrowser` from its option literals; the field no longer
   exists, so a 1.x literal does not compile.
2. Rewrites the template within the profile instead of relying on lowering:
   flex columns become a table or block layout, an absolutely positioned cover
   image becomes an `<img>` with a declared width, border-box sizing becomes the
   default content-box model with explicit padding, `rem`/`vw` become `pt`/`%`,
   custom properties become literals, and cosmetic declarations are removed.
3. Expects `resource/limit` from a document that now spends more of the work
   budget: shaping, line building, stylesheet parsing, font conversion, the
   writer and adjacent-sibling matching are all charged.
4. Expects `html/encoding` for a document that declares windows-1252, even when
   its bytes happen to be valid UTF-8, and `css/unsupported-value` for a
   percentage `height`, a CSS-wide keyword in `font-family` and a
   pseudo-element outside the final compound.
5. Expects a 16-bit PNG to keep its depth, so its PDF is larger than in 1.x,
   and a capped `/BaseFont` for a very long family name.

`CharsetSupported("")` now reports true, because the empty `Options.Charset` is
the valid request "detect the encoding"; a caller that checks only named
encodings writes `name != "" && CharsetSupported(name)`.

### Added

- **Aggregate asset limits.** `Limits` gains `MaxTotalImageBytes` (64 MiB),
  `MaxTotalImagePixels` (80,000,000) and `MaxTotalFontBytes` (32 MiB), so one
  conversion also bounds the totals of its images and fonts, not only each file.
  An asset that would cross a total is rejected with `resource/limit` before it
  is opened or its pixels are decoded, and the fields are additive so a caller
  can raise them.
- **OpenType fonts with CFF outlines.** `@font-face` and `Options.FallbackFont`
  accept an `.otf` whose outlines are PostScript, so a template can name PT Serif,
  PT Sans and the other families that ship that way without converting the font
  first. The engine reads the `CFF ` table and its Type 2 charstrings itself,
  fits quadratics to the cubic outlines in memory within a quarter of a font unit
  and deterministically, and then takes the existing path: shaping, subsetting,
  `CIDFontType2`/`FontFile2` embedding and the copyable text layer. The glyph
  numbering, `cmap`, `GPOS` and the metrics are kept and the PDF structure does
  not change. The conversion is bounded — a glyph over 8192 points or a `glyf`
  over four times the input, never above 64 MiB, fails with `resource/limit` —
  and it refuses rather than approximates the deprecated `seac` accent
  composition. WOFF, WOFF2, CFF2, variable fonts, font collections and remote
  URLs stay rejected, each with a diagnostic naming the format.
- **`html/truncated` diagnostic.** A document that ends inside an HTML tag
  fails with that diagnostic at the position of the incomplete tag, instead of
  converting a document shorter than the author wrote.
- **Input encodings.** A template no longer has to be UTF-8. windows-1251,
  KOI8-R and IBM866 are accepted alongside UTF-8, detected the way the HTML
  standard detects them — a byte order mark, then a `<meta charset>` or
  `http-equiv` declaration in the first 1024 bytes — or named explicitly with
  `Options.Charset` and the reference command's `-charset`, which wins over both.
  A UTF-8 byte order mark is removed instead of becoming a character in the
  body, and a document that uses one now converts to the same bytes as the same
  document without it. Failures name their cause: `html/encoding` for an
  encoding outside the set, a byte the encoding does not define, a UTF-16 input
  and a document that is neither valid UTF-8 nor declared; `css/link-encoding`
  for a linked stylesheet that cannot be decoded or that carries a conflicting
  `@charset`; `options/charset` for an unsupported caller-named encoding,
  reported before any asset is read. A linked stylesheet is decoded with the
  document's encoding, and every diagnostic keeps pointing at the byte, line and
  column of the original file.

### Changed

- **No new runtime dependency for OpenType support.** The CFF reader lives in
  `internal/font`, so the published package's build closure is unchanged;
  `golang.org/x/image`, the decoder the font tests compare against, stays the
  test-only requirement it already was.
- **HTML frontend.** Documents are parsed by `golang.org/x/net/html`, the
  standard tree-construction algorithm maintained by the Go team, instead of the
  in-house tokenizer and tree builder. Character references now follow the HTML
  standard (including legacy names without a semicolon and windows-1252 numeric
  references), and implied elements such as `tbody` appear exactly as a browser
  would produce them. Source positions are unchanged for every element written
  in the document; elements the parser implies inherit the position of their
  nearest ancestor.
- **Dependencies.** The module may now require a module the Go team maintains,
  pinned and covered by `go.sum`. The previous "no dependencies" rule is
  replaced by that allowlist; `golang.org/x/net` is the first requirement
  outside the standard library. The HTML charset package now decodes legacy
  input, so `golang.org/x/net` brings its encoding tables along and the binary
  grows by about 830 KB. `golang.org/x/image` is a test-only requirement: the
  font tests compare the private parser and subsetter with the Go project's
  decoder, `sfnt`, and the module stays out of the package's runtime graph.
- **Text normalization.** `golang.org/x/text/unicode/norm` performs canonical
  composition instead of a generated table and its undocumented generator, so
  the result follows the Unicode version of the module. Canonical decomposition
  now runs first: a precomposed letter followed by a combining mark composes
  where it could not before (`ḋ` plus a dot below becomes `ḍ` plus a dot above).
  The normalization tables add about 137 KB to the binary and replace 43 KB of
  generated source.
- **JPEG headers.** The writer reads width, height and colour components with
  `image/jpeg` instead of a private marker parser, so the writer and the engine
  agree on which files are valid. A JPEG variant the standard library cannot
  decode — 12-bit or lossless — is rejected with a diagnostic naming the file
  instead of being embedded untouched.
- **UTF-16.** PDF strings, `ToUnicode` maps and the debug extraction use
  `unicode/utf16`. The output for characters inside the BMP is unchanged.

### Fixed

- **A spanning cell covers its rows.** The rows a `rowspan` cell covers now sum
  to at least the cell's outer height: when the covered rows are shorter, the
  last one grows by the deficit, so the cell's content cannot paint over the
  block that follows the table. The measured table height, pagination and the
  static path inside a cell share one calculation, and a span that already fits
  keeps its row heights and page count.
- A character outside the BMP in a `ToUnicode` map extracts as one character.
  It used to extract as two replacement characters, because the surrogate pair
  was read back as two separate code points.
- **Forced breaks.** A `break-after: page` followed by a `break-before: page`
  now makes one page transition instead of two, so those documents lose the
  blank page between the two paragraphs and `counter(pages)` counts the pages a
  reader sees. A pending break is taken only from a page that already has body
  content: an empty `<div>` after a break neither cancels it nor creates a page,
  a box that paints only a height or a background satisfies it, and a trailing
  `break-before` on an empty box no longer appends a blank page. Break-before on
  the first content still keeps one page.
- **Table row breaks.** A forced break on a `<tr>` now returns to the page loop
  instead of creating a page inside table placement, so the table background and
  the enclosing fragments are painted on their own page, with its coordinates
  and before the cells. Adjacent row breaks make one transition even when the
  table repeats a header, and a page is never left holding only repeated
  headers.
- **Table measurement.** `break-inside: avoid` measures a table from its built
  grid, so a two-column table moves to the next page whole instead of splitting
  between two rows.
- **Indented lines.** `text-indent` counts toward a line's natural width together
  with a leading inline image, so a right-aligned line with both ends at the
  content edge instead of overflowing by the image width.
- **One overflow warning per box.** A clipped box is reported once: the
  duplicated fixed-height pass is gone, and so is the second report when the text
  path and the fixed height clip the same box.
- **Repeated assets are embedded once.** An image is identified by its canonical
  resolved path after symlinks, or by the SHA-256 of its decoded bytes for a
  `data:` URI, and the PDF writer keys image objects by the same content hash. One
  1.96 MB PNG referenced through 50 spellings used to be read, decoded and
  embedded 50 times, producing a 98 MB PDF; it now produces one object and a PDF
  close to the asset size. Repeated `data:` URIs are decoded once.
- **The work budget covers every stage.** `Limits.MaxOperations` now charges
  stylesheet rules and compounds matched by the cascade, font parsing, image
  decoding, copied drawing operations and diagnostic path segments in addition
  to tokens, boxes, lines and writing operations. A selector that uses the
  adjacent-sibling combinator is matched by splitting it at `+` and running the
  ancestor-chain dynamic program on each segment, so a deep tree costs
  `O(compounds × depth)` instead of combinatorial time; the previous recursive
  path ran 92 seconds on a 2.3 KB document. A conversion that runs out of budget
  fails with `resource/limit` from the stage that hit it, and a cancelled context
  is observed inside matching, image decoding and path building rather than after
  them. Documents within the default limits produce byte-identical output.

## [1.0.0] - 2026-11-25

First release. The engine had been used for printed business documents since
the milestones M0–M4 described below; 1.0.0 fixes the public contract, the
documentation and the compatibility promise around it.

### Added

- **Library API.** `pdfgen.Convert` renders a document and returns the PDF;
  `pdfgen.Validate` runs the same pipeline and reports the same diagnostics
  without producing one. `Options`, `Limits`, `Diagnostic`, `Error` and
  `Result` are the whole public surface.
- **HTML and CSS.** A tokenizer and tree with implicit tags and table
  foster-parenting; a CSS tokenizer, parser, selectors, cascade, specificity,
  inheritance, shorthands and physical longhands; `@page` including
  `landscape`/`portrait`, `@font-face`, print media rules and local
  stylesheets through `<link rel="stylesheet">`.
- **Layout.** Blocks with collapsing margins, inline runs with baseline
  alignment, lists, fixed-layout tables with `colspan`/`rowspan` and collapsed
  borders, images as replaced blocks, and `background-image` with
  `background-size: cover` and clipping.
- **Pagination.** `break-before`/`break-after`/`break-inside`, `orphans` and
  `widows`, repeated `thead`, page counters in margin boxes, and a table of
  contents built with `target-counter(attr(href), page)` in two layout passes.
- **Text and fonts.** TrueType loading, `cmap` formats 4 and 12, `GPOS`
  kerning, line breaking with non-breaking sequences, justification, Unicode
  composition, font subsetting, and an `Identity-H` font with `ToUnicode`, so
  Cyrillic text can be copied and searched out of the PDF.
- **Images.** PNG and JPEG, including JPEG without recompression, `data:` URI
  sources, flattened PNG alpha, and a diagnostic for a colour space the writer
  cannot embed.
- **PDF output.** Valid structure, document metadata, bookmarks from `h1`–`h3`,
  `FlateDecode`, and deterministic `/ID` and `/CreationDate` so the same input
  produces a byte-identical file.
- **Diagnostics.** A stable machine-readable `Code`, a 1-based source
  position, the element's path, every problem found in one pass, and a
  deterministic order.
- **Budgets and cancellation.** `Options.Limits` bounds input size, page count,
  image pixels, font bytes, nesting depth, table grid area, the debug raster
  and total work; a document over a limit fails with a `resource/limit`
  diagnostic instead of exhausting memory. Every expensive stage honours the
  context.
- **Browser import.** An opt-in compatibility layer that lowers a
  browser-oriented template — a single-line flex column, an absolutely
  positioned cover image, `rem`, `vw`/`vh`, `min()`/`clamp()`, CSS variables,
  screen-only `@media`, `text-transform`, `box-sizing` — to the profile and
  reports every approximation as a warning.
- **Reference command.** `pdfgen [-o out.pdf] input.html`, the shortest
  complete program that uses the package.
- **Documentation.** A README for the chooser, package documentation and
  runnable examples for the caller, the CSS profile, font and browser-import
  reference, and `CONTRIBUTING.md` for maintainers.
- **Test infrastructure.** Document snapshots that freeze the parsed model as
  XML, render snapshots that compare every page as a PNG, a corpus of 149
  documents extracted from the mPDF tests, fuzz targets for the parsers, the
  PDF writer and the public entry points, and independent validation of every
  produced PDF with `pdfcpu` in CI.

### Changed

- The module targets Go 1.26 and has no dependencies, so it has no `go.sum`.
- Documentation is organised by audience: the README answers "should I use
  this?", the package documentation answers "how do I call it and what is
  promised?", and contributor material lives in `CONTRIBUTING.md`.
- The reference command is now a minimal example of the package rather than a
  converter with its own flags; the record of that removal is under Removed.
- Resource limits, cancellation and deterministic output apply to every entry
  point, so the CLI and an embedding program behave identically.

### Removed

- The `convert` and `validate` subcommands of the reference command, and the
  flags `-print-profile`, `-render-png`, `-strict`, `-date`, `-page`,
  `-margin`, `-font-dir` and `-base-dir`. The command is an example and
  carries no compatibility promise. `Validate` and `Options.Strict` cover the
  use cases those flags served.

### Fixed

- A truncated PNG is rejected during validation instead of at paint time.
- `rowspan`-shifted columns count against the table cell limit.
- A soft hyphen is an invisible break point instead of a visible character.
- `break-before` and `break-after` are honoured on table rows.
- Each diagnostic is printed once by the reference command.
- A cancelled context is reported before a template error.
- Table rows are placed inside the table padding, `vertical-align` applies to
  inline runs and images, and a collapsed border paints one line.
- The `media` attribute of a `<style>` element is honoured.
- Percentage `line-height` and `em` border widths are computed correctly.
- PNG alpha is flattened once, and glyph widths are rounded identically in
  `/W` and `TJ`.
- Nested tables, hidden table groups, rows and cells, and implicit heading
  scoping render as the HTML scope defines them.
