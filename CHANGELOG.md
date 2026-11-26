# Changelog

All notable changes to this project are documented in this file. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Caller-supplied asset source.** `Options.Source` accepts an `fs.FS` —
  `embed.FS`, `os.DirFS`, `fstest.MapFS` or a subtree made with `fs.Sub` — and
  every document-referenced image, stylesheet and `@font-face` source, together
  with `Options.FallbackFont`/`Options.FallbackFonts`, is then read through it.
  An embedded deployment needs no temporary directory, and the source is
  authoritative: no host file and no installed host font is consulted while it
  is set.
  `BaseDir` and `FontDir` name directories inside the source and default to its
  root; a source path is relative, must stay inside the source, and an absolute
  or escaping reference keeps the existing `img/load`, `css/link-path` or
  `font/unavailable` diagnostic. The field is additive: a nil `Source` keeps
  every existing conversion byte-identical.
- **Streaming conversion.** `pdfgen.ConvertTo(ctx, dst, src, opts)` runs the same
  pipeline as `Convert`, reports the same diagnostics and writes the same bytes,
  but sends the PDF to an `io.Writer` as it is serialised. Serialisation holds at
  most one page's content stream, so the memory needed to serialise the document
  no longer grows with its size. `Result.PDF` is nil for this entry point. A
  layout or validation error is reported before the first byte; a write error
  (reported as `pdf/write`) or a cancelled context can leave a prefix of the PDF
  in `dst`, which the caller must discard.

### Changed

- **BREAKING — asset references are confined by default.** A relative image or
  stylesheet reference resolves against `BaseDir`, then the directory of
  `Options.File`, then the process working directory; a relative `@font-face`
  source resolves against `FontDir`, then `BaseDir`, then the same fallbacks.
  Every relative reference is confined to its root, so a `..` path or a symlink
  that leaves it fails with `img/load`, `css/link-path` or `font/unavailable`
  and produces no PDF. An absolute reference is accepted only inside a
  configured root: with no `BaseDir`/`FontDir`, a path such as `/etc/hosts` is
  no longer read as an image, stylesheet or font, and an absolute `@font-face`
  source is confined to `FontDir`. A relative reference under an absolute
  `Options.File` now resolves against the document's directory instead of the
  process working directory. Migration: set `BaseDir`/`FontDir` to the
  directory that holds the assets, or set the new
  `Options.AllowAbsoluteAssets` to keep reading an absolute reference as
  written. The field is additive and defaults to false; it does not change
  `FallbackFont`/`FallbackFonts`, which are caller-selected paths and stay read
  as written, nor `Options.Source`, whose paths are relative by definition.
- **Style attribute declaration list.** A `style` attribute is now parsed as a
  declaration list that ends only at the end of the attribute. A `{` or `}` in
  it is `css/syntax` and an at-rule is `css/unsupported-at-rule`, instead of the
  brace ending the list silently and the at-rule being discarded; declarations
  before the offending token stay in the cascade and every declaration is still
  validated. This is breaking for a template that relied on the bypass: it
  validates clean today and fails after the change. A well-formed attribute
  produces the same bytes as before.
- **Byte-aware work budget.** The work budget now charges memory as well as CPU:
  every stage whose memory grows with the input charges `ceil(n / 4096) + 1`
  units before it allocates, so one unit stands for at most 4096 bytes of
  materialised input. Line indexes, decoded text, asset reads and offset-bearing
  glyph copies are charged, and a document that is cheap in tokens but allocates
  hundreds of megabytes now stops with `resource/limit`. Documents inside the
  limits keep byte-identical output.
- **Bounded line index.** The HTML and stylesheet line indexes store at most
  65,536 starts and charge each stored entry; a position past the cap scans and
  charges the bytes it reads. A stylesheet made of newlines fails with
  `resource/limit` once the cap is reached instead of allocating one entry per
  newline.
- **Aggregate stylesheet bytes.** `pdfgen.Limits.MaxTotalStylesheetBytes` is a
  new field (default 16 MiB) that bounds the encoded bytes of every linked
  stylesheet in one conversion, in addition to `MaxStylesheetBytes` per file. A
  document that crosses the total fails with `resource/limit` before the next
  sheet is read. The field is additive; a zero value selects the default.
- **Diagnosed preview encoding failure.** A debug preview whose raster is
  produced but whose PNG encoding fails is no longer returned as a silent empty
  `Result.PNG`: the conversion reports a `raster/render` diagnostic naming the
  document, returns an error and leaves `Result.PNG` nil. The PDF bytes are
  unchanged, and a document converted without `Options.RenderPNG` is
  byte-identical.

### Deprecated

- **Debug preview option.** `Options.RenderPNG` is a debug-only preview of the
  first page: only the first page is rendered, at 96 dpi, bounded by
  `Limits.MaxRasterPixels`, and it is not production output. It stays for the
  rest of the 2.x series and may be removed only in the next major release, when
  a debug package or a preview entry point can replace it.

### Removed

- **BREAKING — the implicit system-font search.** With no matching `@font-face`
  and no caller-selected font, conversion used to try four hard-coded system
  paths (`Arial.ttf`, `Times New Roman.ttf`, `DejaVuSans.ttf` and
  `LiberationSerif-Regular.ttf`) and use the first that existed, so the same
  template produced different glyphs, line breaks and PDF bytes on two hosts.
  The search and the exported mutable `DefaultFallbackFonts` slice are removed:
  no installed font is read unless the caller names one, and a missing font is a
  `font/unavailable` diagnostic naming the requested family and both options
  that supply a font. The ordered `Options.FallbackFonts []string` is added next
  to `Options.FallbackFont`, and `pdfgen.SystemFallbackFonts()` returns the
  conventional platform paths as a fresh copy for callers that opt in. The
  reference command passes that list, so `pdfgen invoice.html` keeps working on
  a developer machine while the library default stays reproducible.

### Migration to 3.0.0

A conversion that relied on the implicit search now fails with
`font/unavailable`. There are two ways to restore rendering:

1. **Ship the font with the document.** Declare it with `@font-face`, or set
   `Options.FallbackFont` to one explicit file, or `Options.FallbackFonts` to an
   ordered list. The first list entry that loads becomes the family, with its
   bold and italic siblings discovered next to it. These are caller-selected
   bytes, so the output is identical on every host.
2. **Opt back into the host's fonts.** Set
   `FallbackFonts: pdfgen.SystemFallbackFonts()` to reproduce the old
   platform search. The result is host-dependent by definition; the function
   returns a fresh copy, so modifying or truncating it cannot change another
   call or a concurrent conversion.

Code that read the removed `DefaultFallbackFonts` slice migrates to
`pdfgen.SystemFallbackFonts()`. The `cmd/pdfgen` command already passes it, so it
needs no change.

### Fixed

- **Advance overflow.** Run and line widths are accumulated in 64 bits and
  clamped to `MaxInt32` instead of wrapping, so a 350,000 character word at 12 pt
  keeps a positive width and reports `layout/overflow` once for its paragraph
  again; 1,000 characters kept warning, while the longer word had been clipped
  silently once the `int32` sum turned negative. `text.Scale`, the face metrics
  and the mark-offset conversion round once and clamp to the `int32` range
  before narrowing, so an extreme `font-size` is diagnosed at the element and
  rendered with clamped, non-negative geometry instead of an
  architecture-dependent value. A line exactly at the limit is not reported, and
  documents whose geometry fits keep byte-identical output.
- **Diagnostic columns.** A diagnostic column now counts characters, not bytes,
  in every path that derives it from the source: a leading UTF-8 byte order mark
  is not a character, so adding or removing it never shifts a column, and a
  linked stylesheet and an `html/encoding` error name the column the author sees.
  A single-byte encoding still counts one byte per character. A declaration in a
  stylesheet is reported at its value rather than at the whitespace after the
  colon. No API change and no diagnostic code, severity or message change.
- **Charged GDEF parsing.** GDEF class ranges are now charged to the
  conversion's shared work budget before their mark map grows: one unit per
  class-3 range plus one per entry written, with a guard check between ranges
  and mark-set coverages, so a cancelled context stops a hostile table instead
  of returning a font after the whole parse. Expanded mark entries are capped at
  200,000 writes per font, GDEF mark-set coverages share the existing
  200,000-glyph coverage cap, and a coverage `resource/limit` is propagated
  instead of being swallowed. A truncated GDEF still degrades to "no mark
  data"; fonts within the bounds load byte-identically.
- **Non-regular asset files.** An image, stylesheet or font reference that
  resolves to a directory, device, FIFO, socket or other non-regular file is now
  refused with the resource's existing load diagnostic — `img/load`,
  `css/link-load` or `font/unavailable` — instead of being opened and read. A
  FIFO reference can no longer block the conversion in `open(2)` waiting for a
  writer: the regular-file check runs before the open and again through the
  opened descriptor, and the open is non-blocking on Unix. A symlink to a
  regular file inside the root still reads, while a symlink to a FIFO or to a
  file outside the root is rejected like its target.
- **Table layout defects.** A table's declared column widths are now summed in
  64 bits and scaled against the true total, then clamped so the last column's
  right edge stays inside the content box; the single `layout/overflow` warning
  is emitted only after the clamp and describes the geometry that was produced.
  A word wider than a table cell reports `layout/overflow` once for the cell,
  exactly as the same word in a paragraph does. The forced-break content
  predicate now asks whether a box paints anything: an empty table no longer
  consumes a pending break or appends a blank page, while a table that paints
  only a height or a background (or an image with no visible data but a height,
  background or border) still consumes it. A table whose declared widths fit
  keeps byte-identical layout.
- **Asset admission.** Concurrent reads of one asset identity now share one
  read and one charge: eight readers of one 1 MiB file under a 1.5 MiB total all
  succeed with the file read once and 1 MiB counted, instead of seven being
  refused while the first reservation was pending. The aggregate total is
  charged for the bytes actually admitted, so a file whose reported size differs
  from its contents is reconciled or refused instead of being returned for free,
  and a discarded record refunds exactly the amount it was charged, so totals
  cannot drift or go negative. A failed open is classified by the containment
  check instead of its message, so a mode-000 file named `escapes.css` inside
  the root reports `css/link-load` and only a reference that really leaves the
  root reports `css/link-path`. The document metadata that produces
  `options/title`, `options/author`, `options/subject` and `options/keywords` is
  validated with the other options before any linked asset or font is opened, so
  an invalid title is reported even when a linked stylesheet is missing.
- **Combining marks after a new base.** A combining mark now attaches to the
  base that immediately precedes it instead of reusing an anchor from an earlier
  cluster: in `о́о́` and `а́б́` the second acute is painted over the second
  letter, mark-to-mark stacking only follows a mark of the same cluster, and a
  font without a mark-to-mark anchor falls back to the current base's anchor. An
  anchor that would paint a mark outside its base's cell or outside the shaped
  run reports `text/glyph` instead of drawing it there. Documents whose marks
  already attached correctly are byte-identical.
- **Font table defects.** A GPOS `ScriptList` that names only a non-ordinary
  script now enables no lookup instead of every lookup, so a pair declared under
  Arabic cannot kern Latin text. Crossing the glyph-coverage cap is
  `resource/limit` instead of silently dropping the remaining lookups. A
  composite component id at or beyond `numGlyphs` is refused by the outline
  reader and the subsetter instead of leaving a dangling reference. A truncated
  simple or composite glyph is an error naming the glyph instead of an empty or
  partial outline. Converting CFF to TrueType copies `GDEF`, so mark classes and
  `IgnoreMarks` behaviour survive.
- **Empty inline anchors.** An inline element with an `id` and no content now
  keeps a zero-content inline box and a zero-height carrier line, so its id lands
  in the anchor map on the page it occupies instead of making a `target-counter`
  print 0 and warn `pagination/unknown-target`. The carrier line paints nothing
  and is not page body, so the page and line counts of a document are unchanged.
- **Justified page endings.** The closing line of every page keeps its natural
  width under `text-align: justify` instead of being stretched to the content
  edge; only a paragraph's final line and a line ended by a forced break stay
  unstretched, as before.
- **Table bottom decoration.** A completed table's bottom padding and border are
  clipped to the page content bottom and report `layout/overflow` once, instead
  of painting past the MediaBox when the page is too short for them. The flow
  still advances by the declared insets, so following content starts on the next
  page.
- **Margin box geometry.** Margin box content width and origins are computed with
  saturating arithmetic and clamped into the page; a box whose label cannot be
  placed reports `layout/overflow` naming the box instead of painting at a
  wrapped coordinate.
- **Spanning cell continuation across pages.** A `rowspan` cell that crosses a
  page break now continues every fragment with its text, background and all four
  border sides, and two spanning cells in the same row keep independent
  continuations instead of the later one overwriting the earlier one. A cell
  whose content cannot be fragmented reports `layout/overflow` once instead of
  losing the remaining content silently.
- **Minified source positions.** A position on a UTF-8 document without a byte
  order mark resumes from the last position on its line instead of rescanning the
  line from the start, and the scanned bytes and every path byte are charged to
  the work budget. A one-line document no longer makes diagnostics or layout
  provenance quadratic, and an element with tens of thousands of classes builds
  its path in linear time.
- **Class tokens and no-break spaces.** A class attribute now splits on ASCII
  whitespace only, as HTML defines it, so `class="a&#160;b"` is one token that
  `.a` does not match; the split is cached on the element and reused by every
  selector.
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
- **CSS escapes in selectors and `url()`.** An escaped character in a selector now
  names the character (`.a\.b` is one class named `a.b`) instead of changing the
  selector's structure, and a `url()` argument decodes escapes before it reaches
  the resolver (`url("a\20 b.png")` names `a b.png`).
- **Saturated layout arithmetic.** A box whose padding or declared height is near
  the int32 limit no longer measures a negative height or paints following content
  above itself; measurement, placement and painting saturate, and a box that
  cannot fit reports `layout/overflow` once.
- **Encoding prescan.** Only a real `<meta charset>` or
  `<meta http-equiv="Content-Type">` declaration in the first 1024 bytes decides
  the input encoding. A declaration written inside a comment, a charset
  attribute on another element, or the word `charset` in another attribute's
  value no longer influences detection, so a windows-1251 document with such a
  mention reports its first invalid byte instead of naming an unrelated
  encoding as unsupported. The supported encoding set is unchanged.

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
