# CSS profile

The profile is a closed allow-list. Anything that is not in it is rejected by
validation with a position in the source. The list below is the profile, and
`internal/profile` makes it executable.

## Elements

```
html head body main div p h1 h2 h3 h4 h5 h6
ul ol li table thead tbody tfoot tr td th
img span br hr strong b em i a title meta style link
```

`<script>` is allowed but ignored with the warning `html/ignored-script`: the code is not executed or rendered. With `Options.Strict` this is an error.

An unknown element is the error `html/unsupported-element`. Attributes are
validated too: an attribute outside the documented list is the warning
`html/unsupported-attribute` with its position, which `Strict` turns into an
error.

| Scope | Attributes |
|---|---|
| every element | `id`, `class`, `style`, `hidden`, `lang`, `dir`, `title` |
| `a` | `href` (a local `#id` link; `target-counter` reads it) |
| `img` | `src`, `alt`, `width`, `height` |
| `link` | `rel`, `href`, `media` |
| `meta` | `name`, `content`, `charset`, `http-equiv` |
| `td`, `th` | `colspan`, `rowspan` |
| `ol` | `start` |

The HTML `hidden` attribute hides the element's subtree like `display: none`;
its content is not laid out and does not appear in the PDF.

## Input encodings

The input does not have to be UTF-8. The encoding is detected as the HTML
standard detects it:

1. a byte order mark, which also decides between UTF-8 and UTF-16;
2. a `<meta charset>` or `<meta http-equiv="Content-Type">` declaration in the
   first 1024 bytes;
3. otherwise UTF-8.

A UTF-8 byte order mark is removed before parsing. A declaration overrides the
bytes: a document that says `windows-1251` is read as windows-1251 even when its
bytes happen to be valid UTF-8, exactly as a browser would read it.

The supported set is closed: `utf-8`, `windows-1251`, `koi8-r` and `ibm866`. A
declaration of anything else — `shift_jis`, `windows-1252`, a UTF-16 byte order
mark — fails with `html/encoding` naming the encoding, and the position of the
first `<meta>` in the window when there is one. windows-1252 is deliberately not
supported: the standard library reports it both for a document that declares it
and for a document that declares nothing and is not valid UTF-8, so accepting it
would read an undeclared windows-1251 document as Latin mojibake. Convert such a
file, or name the real encoding.

A document that is not valid UTF-8 and declares nothing fails with
`html/encoding` at its first invalid byte, and a byte the selected encoding does
not define (windows-1251 `0x98`) fails at that byte. Bytes are never replaced
with U+FFFD.

`Options.Charset` (`-charset` in the reference command) names the encoding
explicitly and wins over the mark and the declaration; an unsupported value is
`options/charset`, reported before any asset is read. It is the way to convert a
document that declares nothing.

A position inside a decoded document or stylesheet names a line and a character
column: the column counts characters, not bytes, so a diagnostic after a
Cyrillic word names the column the author sees in an editor — in a UTF-8
document and in a legacy one alike.

## Properties

| Property | Inherited | Values |
|---|---|---|
| `display` | no | `block`, `inline`, `list-item`, `table`, `table-row`, `table-cell`, `none` |
| `width`, `height` | no | a length or `auto` |
| `margin`, `padding` | no | 1–4 lengths |
| `margin-top`, `margin-right`, `margin-bottom`, `margin-left` | no | a length; `auto` is rejected |
| `padding-top`, `padding-right`, `padding-bottom`, `padding-left` | no | a non-negative length |
| `border` | no | `[width] [solid\|none] [color]` |
| `border-collapse` | no | `collapse`, `separate` |
| `background-color` | no | a colour |
| `background-image` | no | `url(<local file>)` or `url("data:image/png;base64,…")`, `url("data:image/jpeg;base64,…")` |
| `background-size` | no | `cover` |
| `color` | yes | a colour |
| `font-family` | yes | a list of families |
| `font-size` | yes | a length, `em`, `%` |
| `font-weight` | yes | `100`–`900`, `normal`, `bold` |
| `font-style` | yes | `normal`, `italic` |
| `line-height` | yes | a number, a length, `normal` |
| `text-align` | yes | `left`, `right`, `center`, `justify` |
| `text-indent` | yes | a length |
| `white-space` | yes | `normal`, `pre` |
| `vertical-align` | no | `baseline`, `top`, `middle`, `bottom` |
| `break-before`, `break-after` | no | `auto`, `page`, `always` (an alias of `page`), `avoid` |
| `break-inside` | no | `auto`, `page`, `avoid` |
| `page-break-before/after/inside` | no | legacy aliases of `break-*` |
| `orphans`, `widows` | yes | an integer ≥ 1 |
| `list-style-type` | yes | `disc`, `decimal` |

## Units

`px`, `pt`, `mm`, `cm`, `in`, `%`, `em`. Everything else (`rem`, `vw`, `vh`, `ch`) is an error.

A percentage resolves against the containing block width for `width`, margins, padding
and `text-indent`; `em` resolves against the element's computed `font-size`.

With `border-collapse: collapse` the shared grid line is drawn once: the outer border comes from the table's own `border` (if set), and the inner borders are the left and top borders of the cell to the right or below. A conflict between different widths and colours is resolved in favour of that single border; CSS priorities (`cell > row > table`) are not modelled.

The soft hyphen `U+00AD` (`&shy;`) is an invisible break point: the engine does not synthesise a visible hyphen at the break. A regular `-` and `U+2010` break the line after themselves and are preserved in the text layer as written.

`auto` is allowed only for `width` and `height`. For `margin`, `padding`, `text-indent` and `line-height` it is a profile error: `margin: 0 auto` does not centre the block, whereas it used to silently align it to the left.

Quoted CSS strings decode backslash escapes: one to six hexadecimal digits followed by
an optional space, a backslash-newline continuation, and a backslash before any other
character. `content` and `font-family` see the decoded text, so `"\2014"` is an em
dash. The `border` shorthand accepts every colour form `color` accepts, including
`rgb()` and hexadecimal values; a `transparent` border keeps its layout width but paints
no pixels.

A background image is painted behind the box's content, scaled to cover the box and clipped to it. The same `data:` URI forms as for `<img src>` are accepted; remote URLs are not. `cover` is the only supported value; the initial `auto` behaves as `cover`, because the profile has no natural-size painting, so a background image always fills its box. The crop is centered: there is no `background-position`, `background-repeat` or multiple-background grammar. An inline box paints no background at all, neither a colour nor an image; the supported backgrounds belong to block boxes and table cells.

## Pagination

`break-before: page` and `break-after: page` are honoured once at a boundary. When both meet between two boxes — a `break-after` on one and a `break-before` on the next — the engine makes one page transition, for block boxes and table rows alike. A pending break is taken only when the page already carries body content: at the top of a page, or after a box that paints nothing, the boundary is already fresh, so the break is dropped instead of adding an empty page. A box that paints only a background, a border or a declared height is content and satisfies the break; a trailing forced break never appends an empty page. Repeated table headers are not body content, so a break decided on a continuation page is not taken twice. A break on a `<tr>` is handled by the page loop: the table background and the enclosing fragments are finished on the page they belong to before the next page is painted.

`break-inside: avoid` measures a table from its built grid, so the height it compares is the height the table occupies.

`text-indent` shifts the first line of a block and counts toward that line's natural width, together with every inline image on it — including an image before the first word — so alignment and the wrap decision use the real width.

## Selectors

Element name, class, ID, the universal `*`, combinations, descendant (space), direct child (`>`) and adjacent sibling (`+`), and comma-separated lists.

Pseudo-classes and attribute selectors are rejected, with one exception: `:root` matches the document element. The only pseudo-element is `a::after`, for generated content:

```css
a.toc::after { content: " →" target-counter(attr(href), page) }
```

Inside `a::after` only the `content` property is allowed, with quoted strings and
`target-counter(attr(href), page)` — the page the link points at. A page counter
inside `a::after` is rejected with `css/unsupported-value` (it would name the page
the link is on, not its target). Outside `a::after` there is no `content`
property.

## At-rules

```css
@page {
  size: A4;              /* A4, A5, Letter, two lengths or landscape/portrait */
  margin: 20mm 15mm;     /* one to four non-negative lengths */
}

@page { size: A4 landscape }  /* 297 x 210 mm */
@page { size: landscape }     /* orient the page size from Options.PageSize (A4 by default) */
@page { size: portrait }

@page {
  @top-center    { content: "Счёт № 42" }
  @bottom-center { content: "Стр. " counter(page) " из " counter(pages) }
}

@font-face {
  font-family: "PT Serif";
  src: url(fonts/pt-serif.ttf);  /* local TrueType or OpenType/CFF */
}

/* External stylesheet: a local file only */
<link rel="stylesheet" href="styles/print.css">
<link rel="stylesheet" href="styles/screen.css" media="print">

@media print { /* or @media all */ }
```

A page margin is a non-negative length only: `auto`, percentages and negative values
are rejected with `css/unsupported-value`. An `em` margin resolves against the initial
font size (12 pt), not the page width. `size: portrait` flips a landscape fallback and
`size: landscape` flips a portrait one.

Margin boxes: `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, `bottom-right`.
`content` values: quoted strings, `counter(page)` and `counter(pages)`.
`target-counter` inside a margin box is rejected with `css/unsupported-value`,
because a margin box runs before the target pages are known and would either
print the wrong number or force an unbounded second pass for every page.

At-rules may nest at most 64 levels deep — `@media` inside `@media`, a margin box
inside `@page`, and every other nested at-rule. A deeper stylesheet is rejected
with `resource/limit` during parsing, before any walk over the rules.

The nesting policy is explicit: `@media` accepts another `@media`; `@page`
accepts the six margin boxes; every other nesting — an at-rule inside
`@font-face`, a media rule inside a margin box, `@page` inside `@page` — is
reported as `css/unsupported-at-rule` instead of being dropped or hoisted. The
same policy is implemented once, in the parser's `parseAtRule`.

Media queries, CSS-wide keywords, escapes and at-rule nesting each have one
implementation used by both stylesheet collection and validation:
`css.MediaAppliesToPrint` parses a media list, `css.isCSSWideKeyword` decides the
shared keywords (`inherit`, `initial`, `unset`, `revert`, rejected by every
property that cannot implement them, including `font-family`), and the
tokenizer's escape decoder is the only place a backslash sequence is decoded, for
strings, identifiers and hashes alike.

## What is missing and why

| Not supported | Reason |
|---|---|
| `flex`, `grid` | a different layout engine; blocks and tables are enough for documents. |
| `float`, `position: absolute/fixed` | breaks pagination and requires a different algorithm. |
| gradients, shadows, rounded corners, transparency | not needed in printed documents; they complicate the output |
| `background-position`, `background-repeat`, `background-size: contain/auto/lengths` | the cover model is one scaling rule with a centered crop; a positioning grammar is a larger surface for a case no document needs yet |
| `hyphens: auto` | requires hyphenation dictionaries; a known gap, not an active change |
| JavaScript | the engine is static; the document is not executed |
| web fonts (WOFF, WOFF2) and fonts by URL | see `docs/fonts.md` |
| external CSS by URL | the engine does not access the network; `<link>` accepts only a local file |
| `table-layout: auto` | the cost of the automatic table layout algorithm outweighs its benefit |
| `size: 210mm 297mm landscape` | the CSS grammar does not allow an orientation combined with lengths |
| other pseudo-elements (`::before`, `::first-line`) | generated content is needed only for the table of contents |

## External stylesheets

`<link rel="stylesheet">` includes a local file. Rules:

- the path is resolved relative to the run's `BaseDir`; when `BaseDir` is set, going outside it is forbidden;
- `media` on `<link>` and `<style>` with a value that contains `print` or `all`, or with no value at all, applies the sheet; any other list (one without `print`) skips it without an error, the same way a browser skips it for print;
- a `rel` other than `stylesheet` (`icon`, `canonical`) is silently ignored;
- `http(s)://` and `data:` are rejected: remote stylesheets are not loaded;
- the sheet is decoded with the document's encoding (CSS 2.1 gives a linked
  sheet without its own `@charset` the encoding of the referring document);
- a `@charset` rule at the very start of the file is accepted and ignored when
  it names the document's encoding, and is `css/link-encoding` when it names
  another one; a byte the encoding does not define is `css/link-encoding` at
  that byte;
- the cascade order is the document order, so `<style>`, `<link>`, `<style>` apply in exactly that sequence;
- an error inside an included file names the file, the line and the column in it.

A configured `BaseDir` or `FontDir` that cannot be opened fails with
`options/asset-root` before any asset is read; it never falls back to an
unbounded root. The reference command refuses an output that names the input
file, rejects an input larger than `pdfgen.DefaultLimits().MaxInputBytes` before
reading it, and writes through a temporary file renamed into place.

## Known gaps

Two features are documented gaps rather than planned work, so a reader knows
where they stand:

- `text-decoration` (`u`, `s`, `del`, `ins`) and `<blockquote>`: the mPDF port
  found them missing. They are outside the closed profile as it stands and are
  not covered by an active OpenSpec change; adding one means a requirement, a
  default rule and a golden, not a local edit.
- `hyphens: auto`: needs hyphenation dictionaries and a hyphenation pass; no
  active change covers it.

Everything else that is open work lives as an OpenSpec change under
`openspec/changes/`, and `TODO.md` records the review passes and the fixtures
they left behind.

## Nesting depth

`Options.Limits.MaxDOMDepth` (256 by default) bounds element nesting. The depth is
measured on the tree the HTML standard's construction algorithm produces, so implied
elements (`html`, `body`, an implied `tbody`) and implied end tags count, and 300
sibling paragraphs with omitted end tags stay a shallow tree. A document whose deepest
element is over the limit fails with `resource/limit` before the conversion model or
layout allocates.

The token pass may reject a document even earlier when it can prove the limit is
exceeded without the tree: a run of start tags that never close implicitly (`div`,
`ul`, `ol`, `span`, `b`, `strong`, `em`, `i`) and is not interrupted by an end tag or
by table or foreign content. The rule is a lower bound, so a document it does not
reject is decided on the tree rather than on an approximation. A failure of the
standard tree construction is reported as `resource/limit` for its open-stack limit and
as `html/parse` otherwise; neither escapes as a raw error or a panic.

## Diagnostics

A diagnostic decided from the document carries the file, the line, the
character column and, when an element owns the problem, the enclosing element
path:

```
invoice.html:12:5: error[css/unsupported-value]: display: unsupported display value "flex" (at body > div.invoice > p)
```

Three classes of location exist, and a code belongs to exactly one of them:

- **Source-attributable** — the document names the cause: an element, a
  declaration, an attribute, or a referenced asset such as a missing image or
  font file. The diagnostic carries `File`, `Line` and `Column`, and `Element`
  when an element owns the problem. A rule in a linked stylesheet uses the
  stylesheet's own file and position, not the referring document's.
- **Option errors** — `options/*` values name the option. The file and the
  position may be empty, because the problem is in the caller's configuration
  rather than in the document.
- **Aggregate or output failures** — a conversion-wide budget or a writer or
  rasterizer failure that has no single source node names the document and
  leaves the coordinates zero:

  ```
  invoice.html: error[resource/limit]: work limit exceeded: ...
  ```

  Zero `Line` or `Column` means unknown, and it is used only when no source node
  identifies the problem. It never stands for a location the source does
  provide: a known position is not discarded and reclassified as unknown.

A column counts characters, not bytes (see "Input encodings").

The catalogue below is generated from the registry in `internal/diag/codes.go`;
a code cannot be emitted or documented without the other side, because a test
compares this table with the registry.

<!-- diagnostics:start -->
| Code | Meaning |
|---|---|
| `css/incomplete-font-face` | an @font-face rule is missing font-family or src |
| `css/link-encoding` | a linked stylesheet cannot be decoded or declares a conflicting @charset |
| `css/link-load` | a linked asset cannot be read |
| `css/link-path` | a stylesheet reference cannot be resolved |
| `css/link-remote` | a remote stylesheet is referenced |
| `css/link-unsupported` | a data: stylesheet is referenced |
| `css/syntax` | a stylesheet or a declaration list is not valid CSS for the profile |
| `css/unsupported-at-rule` | an at-rule or a nesting the profile cannot represent |
| `css/unsupported-margin-box` | a page margin box the profile does not implement |
| `css/unsupported-media` | a media query list that does not apply to print |
| `css/unsupported-page-selector` | a page selector other than the plain @page |
| `css/unsupported-property` | a property outside the profile |
| `css/unsupported-selector` | a selector the profile does not implement |
| `css/unsupported-value` | a value or declaration the profile rejects |
| `font/unavailable` | a font file cannot be loaded or parsed |
| `html/encoding` | the document declares or uses an encoding outside the accepted set |
| `html/ignored-script` | a script element is ignored |
| `html/invalid-span` | a colspan or rowspan outside the allowed range |
| `html/no-body` | the document has no body element |
| `html/parse` | the HTML tree construction failed |
| `html/truncated` | the document ends inside a tag |
| `html/unsupported-attribute` | an attribute the profile does not read |
| `html/unsupported-element` | an element outside the profile |
| `img/load` | an image cannot be loaded or decoded |
| `img/no-loader` | the conversion was asked to lay out an image without a loader |
| `layout/loop` | pagination cannot make progress because of a layout cycle |
| `layout/no-progress` | content cannot be placed and pagination cannot advance |
| `layout/overflow` | content is wider or taller than its box and is clipped |
| `options/asset-root` | a configured asset directory cannot be opened |
| `options/author` | the author metadata is not valid UTF-8 |
| `options/charset` | the caller-named encoding is not supported |
| `options/date` | the creation date is outside the representable range |
| `options/keywords` | the keywords metadata is not valid UTF-8 |
| `options/margin` | the page margin cannot be parsed |
| `options/page-size` | the page size cannot be parsed |
| `options/subject` | the subject metadata is not valid UTF-8 |
| `options/title` | the title metadata is not valid UTF-8 |
| `page/margin-box` | a page margin box names a content value it cannot render |
| `pagination/target-counter` | a target-counter refers to a target it cannot resolve |
| `pagination/unknown-target` | a link refers to an id the document does not contain |
| `pdf/write` | the PDF writer failed for a reason other than the work budget |
| `raster/render` | the rasterizer could not render a page |
| `resource/limit` | a work, page, asset or structural limit stopped the conversion |
| `text/font` | no font face could be resolved for a family |
| `text/glyph` | a character has no glyph or a mark cannot be attached |
<!-- diagnostics:end -->

## Work budget

`Options.Limits.MaxOperations` bounds the total work of one conversion, not only
the parsed tokens. Each stage states what one unit means, and the suite keeps an
invariant test that a hostile fixture at `MaxOperations = N` spends work and
memory within a constant multiple of `N`:

| Stage | One unit is |
|---|---|
| HTML parsing | one element or one text node |
| CSS tokenizing and parsing | one token, one rule or declaration, one stored problem |
| Cascade | one compound of a rule's selector, per element |
| Font parsing | one GiB/1024 of file, one GPOS lookup, subtable, pairing or coverage entry, one charstring instruction or converted glyph |
| Text shaping and line building | one rune, one built line |
| Image decoding | one image plus one unit per 64 KiB of pixels |
| Layout | one box, one placed line, one drawing operation, one table grid slot |
| Diagnostics | one path segment of a diagnostic element path |
| PDF writing | one written glyph, CID, page operation, or 4 KiB of serialised bytes |

The first stage to exhaust the budget stops with `resource/limit`, and a
cancelled context is returned as `context.Canceled` from that stage instead of
being reported as a document diagnostic. A stage that allocates per input unit
without charging the guard is a defect, and the invariant test in
`internal/engine/budget_stages_test.go` fails when a fixture's memory grows out
of proportion to its budget. The default is 20,000,000 operations.

## Asset limits

`Options.Limits` bounds the assets of one conversion in addition to each file.
`MaxImagePixels` (40,000,000) and `MaxImageBytes` (32 MiB) bound one image;
`MaxFontBytes` (16 MiB) bounds one font;
`MaxTotalImageBytes` (64 MiB) and `MaxTotalImagePixels` (80,000,000) cap every
image one conversion reads; `MaxTotalFontBytes` (32 MiB) caps every font it
parses. A document that would cross a total fails with `resource/limit` before
the offending asset is opened or its pixels are decoded.

An image is identified by its canonical resolved path after symlinks, or by the
SHA-256 of its decoded bytes for a `data:` URI. Every spelling of one file and
every repetition of one `data:` URI share one decode and one image object in the
PDF, and the aggregate totals count that identity once.

An option error, such as `options/charset` or `options/page-size`, names no
position: it is about the call, not the document.
