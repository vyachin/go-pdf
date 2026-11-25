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

An unknown element is the error `html/unsupported-element`. The supported attributes are `id`, `class`, `style`, `src`, `colspan`, `rowspan`, `start` (for `ol`), `name`/`content` (for `meta`).

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

A position inside a decoded document or stylesheet is reported in the bytes of
the original file: a diagnostic after a Cyrillic word on one line names the
column the author would see, not the longer UTF-8 column.

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
| `break-before`, `break-after` | no | `auto`, `page`, `avoid` |
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

Inside `a::after` only the `content` property is allowed, with strings, `counter(page)`, `counter(pages)` and `target-counter(attr(href), page)`. Outside `a::after` there is no `content` property.

## At-rules

```css
@page {
  size: A4;              /* A4, A5, Letter, two lengths or landscape/portrait */
  margin: 20mm 15mm;     /* one to four non-negative lengths */
}

@page { size: A4 landscape }  /* 297 x 210 mm */
@page { size: landscape }     /* orient the size from -page (A4 by default) */
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
`content` values: quoted strings, `counter(page)`, `counter(pages)`, `target-counter(attr(href), page)`.

At-rules may nest at most 64 levels deep — `@media` inside `@media`, a margin box
inside `@page`, and every other nested at-rule. A deeper stylesheet is rejected
with `resource/limit` during parsing, before any walk over the rules.

## What is missing and why

| Not supported | Reason |
|---|---|
| `flex`, `grid` | a different layout engine; blocks and tables are enough for documents. The browser import lowers a single-line flex column to block layout. |
| `float`, `position: absolute/fixed` | breaks pagination and requires a different algorithm. The browser import lowers a full-box cover image to a background. |
| gradients, shadows, rounded corners, transparency | not needed in printed documents; they complicate the output |
| `background-position`, `background-repeat`, `background-size: contain/auto/lengths` | the cover model is one scaling rule with a centered crop; a positioning grammar is a larger surface for a case no document needs yet |
| `hyphens: auto` | requires hyphenation dictionaries; planned for v2 |
| JavaScript | the engine is static; the document is not executed |
| web fonts (WOFF, WOFF2) and fonts by URL | see `docs/fonts.md` |
| external CSS by URL | the engine does not access the network; `<link>` accepts only a local file |
| `table-layout: auto` | the cost of the automatic table layout algorithm outweighs its benefit |
| `size: 210mm 297mm landscape` | the CSS grammar does not allow an orientation combined with lengths |
| other pseudo-elements (`::before`, `::first-line`) | generated content is needed only for the table of contents |

## External stylesheets

`<link rel="stylesheet">` includes a local file. Rules:

- the path is resolved relative to the run's `BaseDir`; when `BaseDir` is set, going outside it is forbidden;
- `media` on `<link>` and `<style>` with the value `print`, `all` or empty applies the sheet; any other value (including a list without `print`) skips it without an error;
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

Every error contains the file, line, column and element path:

```
invoice.html:12:5: error[css/unsupported-value]: display: unsupported display value "flex" (at body > div.invoice > p)
```

Codes: `css/unsupported-property`, `css/unsupported-value`, `css/unsupported-selector`, `css/unsupported-at-rule`, `css/unsupported-media`, `css/unsupported-margin-box`, `css/incomplete-font-face`, `css/link-encoding`, `html/unsupported-element`, `html/truncated`, `html/ignored-script`, `html/invalid-span`, `html/encoding`, `html/parse`, `text/glyph`, `layout/overflow`, `resource/limit` (a work or resource bound, such as at-rule nesting).

## Work budget

`Options.Limits.MaxOperations` bounds the total work of one conversion, not only
the parsed tokens. One operation is charged for every stylesheet rule evaluated
by the cascade (once per compound in its selector), every GPOS lookup, subtable
and coverage entry read from a font, every 64 KiB of decoded image pixels, every
layout box, placed line, copied drawing operation and diagnostic path segment
built, and every PDF writing operation. Text shaping is bounded by the same
token and line charges.

The first stage to exhaust the budget stops with `resource/limit`, and a
cancelled context is returned as `context.Canceled` from that stage instead of
being reported as a document diagnostic. The default is 20,000,000 operations.

## Asset limits

`Options.Limits` bounds the assets of one conversion in addition to each file.
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
