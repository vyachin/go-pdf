# css-profile Specification

## Purpose

Defines which subset of HTML and CSS the engine is required to understand, and how it reports what is unsupported. The profile is a contract: a template is either valid as a whole or receives a diagnostic before rendering begins.

## Requirements

### Requirement: The profile is a closed allow-list

The engine SHALL accept only the elements, properties, values, units and at-rules listed in the v1 profile. Any unsupported property, value, unit or at-rule SHALL produce a validation error rather than being silently ignored.

#### Scenario: Unsupported display value
- **WHEN** the template contains `div { display: flex }`
- **THEN** validation fails with the property, the value, the file and the position named
- **AND** rendering does not start

#### Scenario: Unsupported property
- **WHEN** the template contains `box-shadow: 0 0 4px #000`
- **THEN** validation fails, stating that the property is not part of the v1 profile

#### Scenario: A fully supported template
- **WHEN** every element and property of the template is part of the profile
- **THEN** validation succeeds without diagnostics

### Requirement: Supported elements

The profile SHALL include the elements `html`, `head`, `body`, `main`, `div`, `p`, `h1`–`h6`, `ul`, `ol`, `li`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `td`, `th`, `img`, `span`, `br`, `hr`, `strong`, `b`, `em`, `i`, `u`, `s`, `del`, `ins`, `blockquote`, `a`, `link` (only as a stylesheet reference) and `script` (only to be ignored). An unknown element SHALL be rejected with its name and position named.

#### Scenario: An accepted element

- **WHEN** the template contains an `h2` element
- **THEN** validation accepts the element and layout applies the engine stylesheet's default styles to it

#### Scenario: A document landmark

- **WHEN** the template contains a `main` element
- **THEN** validation accepts the element and layout gives it block layout like a `div`

#### Scenario: An unknown element

- **WHEN** the template contains a `video` element
- **THEN** validation fails with the element name and its position named

#### Scenario: Referencing a stylesheet

- **WHEN** the template contains `<link rel="stylesheet" href="styles/print.css">`
- **THEN** validation accepts the element and the styles from the file take part in the cascade

#### Scenario: A link that is not a stylesheet

- **WHEN** the template contains `<link rel="icon" href="favicon.ico">`
- **THEN** validation emits no diagnostics and the element does not affect layout

#### Scenario: A decoration element

- **WHEN** the template contains `u`, `s`, `del` or `ins`
- **THEN** validation accepts each element and the engine stylesheet underlines `u` and `ins` and strikes through `s` and `del`

### Requirement: Supported box model properties

The profile SHALL include the properties `display`, `width`, `height`, `margin`, `margin-top`, `margin-right`, `margin-bottom`, `margin-left`, `padding`, `padding-top`, `padding-right`, `padding-bottom`, `padding-left`, `border`, `border-collapse`, `background-color`, `background-image` and `background-size`. Property values outside the profile SHALL be rejected.

#### Scenario: Border and padding
- **WHEN** the template sets `padding: 4mm` and `border: 1pt solid #000`
- **THEN** validation accepts both declarations and layout applies them to the box

#### Scenario: A physical longhand after the shorthand
- **WHEN** the template sets `margin: 1mm 2mm 3mm 4mm` and then `margin-top: 5mm`
- **THEN** validation accepts both declarations and the top margin is 5 mm while the other sides keep their shorthand values

#### Scenario: A negative padding longhand
- **WHEN** the template sets `padding-left: -1mm`
- **THEN** validation fails with the property and the value named

#### Scenario: Unsupported display value
- **WHEN** the template sets `display: grid`
- **THEN** validation fails with the unsupported value named

#### Scenario: A background image that covers the box
- **WHEN** the template sets `background-image: url(cover.jpg)` and `background-size: cover`
- **THEN** validation accepts both declarations and layout paints the image behind the box's content

#### Scenario: A background image in a data URI
- **WHEN** the template sets `background-image: url("data:image/png;base64,…")`
- **THEN** validation accepts the declaration and the decoded image is painted behind the box's content

#### Scenario: A gradient background
- **WHEN** the template sets `background-image: linear-gradient(#000, #fff)`
- **THEN** validation fails with the property and the value named, and rendering does not start

#### Scenario: An unsupported background size
- **WHEN** the template sets `background-size: contain`
- **THEN** validation fails with the property and the value named

### Requirement: Border colours accept the values color accepts

The `border` shorthand SHALL accept every colour form that the `color` property
accepts, including `rgb()` and hexadecimal values. `transparent` SHALL paint the
border with no visible pixels instead of black.

#### Scenario: An rgb border

- **WHEN** a rule declares `border: 1px solid rgb(255, 0, 0)`
- **THEN** validation accepts the declaration and the border is red

#### Scenario: A transparent border

- **WHEN** a rule declares `border: 1px solid transparent`
- **THEN** no visible border is painted

### Requirement: Supported text properties

The profile SHALL include the properties `color`, `font-family`, `font-size`, `font-weight`, `font-style`, `line-height`, `text-align`, `text-indent`, `white-space`, `vertical-align` and `text-decoration`. Property values outside the profile SHALL be rejected.

#### Scenario: Typeface and size

- **WHEN** the template sets `font-family: "PT Serif"`, `font-size: 12pt` and `font-weight: 700`
- **THEN** validation accepts the declarations and the text is rendered in the specified typeface

#### Scenario: Unsupported white-space value

- **WHEN** the template sets `white-space: pre-wrap`
- **THEN** validation fails with the unsupported value named

#### Scenario: A decoration value

- **WHEN** the template sets `text-decoration: underline`, `none`, `line-through` or `underline line-through`
- **THEN** validation accepts the declaration and every other keyword is rejected with the value named

#### Scenario: A decoration is inherited

- **WHEN** a decorated element contains a child element with no decoration of its own
- **THEN** the child's text carries the same decoration

### Requirement: CSS string escapes are decoded

The CSS tokenizer SHALL decode backslash escapes in strings and identifiers: one to
six hexadecimal digits followed by an optional space, and a backslash-newline
continuation. The decoded value MUST be used by `content` and `font-family`; a
sequence such as `"\2014"` MUST NOT reach the renderer as the characters `2014`.

#### Scenario: Hexadecimal escapes in content

- **WHEN** `a::after { content: "\2014" }` is converted
- **THEN** the generated text is an em dash

#### Scenario: Escaped space

- **WHEN** the value is `"A\20 B"`
- **THEN** the decoded text is `A B`

### Requirement: Supported break and list properties

The profile SHALL include the properties `break-before`, `break-after`, `break-inside`, `page-break-before`, `page-break-after`, `page-break-inside`, `orphans`, `widows` and `list-style-type`.

#### Scenario: Prohibiting a break in the template
- **WHEN** the template sets `page-break-inside: avoid` for a table row
- **THEN** validation accepts the declaration and passes it to pagination

#### Scenario: Unsupported list-style-type value
- **WHEN** the template sets `list-style-type: lower-roman`
- **THEN** validation fails with the unsupported value named

### Requirement: Supported units of measurement

The engine SHALL support the units `px`, `pt`, `mm`, `cm`, `in`, `%` and `em`.
Percentages SHALL resolve against the containing block width, and `em` against the
element's computed `font-size`. The units `rem`, `vw`, `vh` and `ch` SHALL be
rejected as unsupported.

#### Scenario: Mixed units
- **WHEN** the template sets `width: 100mm` and `padding: 1.5em` with `font-size: 12pt`
- **THEN** the widths are converted to a single internal unit without loss of precision
- **AND** the block width equals 100 mm

#### Scenario: Unsupported unit
- **WHEN** the template contains `margin: 2rem`
- **THEN** validation fails with the `rem` unit named

#### Scenario: A percentage width with a percentage margin

- **WHEN** a block declares `width: 50%; margin-left: 10%` in a containing block of
  481,890 millipoints
- **THEN** the border box is 240,945 wide and starts 48,189 from the containing
  block's left edge

#### Scenario: Percentage padding and bottom margin

- **WHEN** a box declares `padding: 0 10%` or `margin-bottom: 10%`
- **THEN** both resolve against the containing block width, not the box's own width

#### Scenario: Percentage text indent

- **WHEN** a block with a 100 pt content width sits in a 481,890 millipoint
  containing block and declares `text-indent: 10%`
- **THEN** the first line starts 48,189 millipoints in, not 10,000

### Requirement: Cascade order and inheritance

The engine SHALL resolve style conflicts in this order: the browser's default styles, then author styles in ascending order of selector specificity, then the `style` attribute. With equal specificity the declaration made later SHALL win. Shorthand properties SHALL expand into longhands.

#### Scenario: An inline style beats a class selector
- **WHEN** a `td` has a class with `color: red` and a `style="color: blue"` attribute
- **THEN** the resulting text colour is blue

#### Scenario: A later rule at equal specificity
- **WHEN** two rules of the same specificity set different `font-size` values
- **THEN** the value from the rule declared last applies

### Requirement: Supported selectors

The engine SHALL support selectors by element name, class and identifier, the universal `*`, their combinations, descendant (space), direct child (`>`), adjacent sibling (`+`), as well as comma-separated lists. Pseudo-classes and attribute selectors SHALL be rejected as unsupported, with the single exception of `:root`, which matches the document element. The single exception for pseudo-elements is described in a separate requirement.

#### Scenario: A combined selector
- **WHEN** the template contains `.invoice > td.total`
- **THEN** the rule applies to cells with class `total` inside a row with class `invoice`

#### Scenario: Adjacent siblings
- **WHEN** the template contains `p + p`
- **THEN** the rule applies to every `p` that immediately follows another `p` among the element siblings

#### Scenario: The document element
- **WHEN** the template contains `:root { color: #333 }`
- **THEN** the rule applies to the `html` element and not to its descendants

#### Scenario: Unsupported pseudo-class
- **WHEN** the template contains `tr:nth-child(2n)`
- **THEN** validation fails with the unsupported selector named

#### Scenario: Unsupported pseudo-element
- **WHEN** the template contains `p::before { color: red }`
- **THEN** validation fails, stating that only `a::after` is supported

### Requirement: Selector matching is bounded and accounted

Matching a supported selector against an element SHALL take time bounded by the number of compounds and the element's ancestor depth, including selectors that use the adjacent-sibling combinator `+`. The work of matching MUST be recorded in the operation budget, and a conversion whose budget is exhausted or whose context is cancelled while a selector is matched MUST stop with the documented `resource/limit` diagnostic or context error.

#### Scenario: Adjacent selector on a deep tree
- **WHEN** a stylesheet contains a selector with three or more compounds that includes
  `+`, and the document nests elements deeper than the selector's descendant count
- **THEN** conversion finishes instead of spending time exponential in the depth
- **AND** the document renders with the styles the selector matches

#### Scenario: Matching work is counted
- **WHEN** a document requires more selector-matching work than its operation budget
  allows
- **THEN** the call stops with a `resource/limit` diagnostic

#### Scenario: Cancellation during matching
- **WHEN** the context is cancelled while a selector is matched against an element
- **THEN** the call returns the context error
- **AND** the error is not turned into a document diagnostic

#### Scenario: Results do not change
- **WHEN** an adjacent selector is matched against a document
- **THEN** the matched elements are exactly those the unoptimised matcher would return

### Requirement: Diagnostics are tied to their source

Every diagnostic SHALL carry the file, line, column and the element's path in the document tree. The engine SHALL report every error it finds in a single validation pass, not only the first. The order of diagnostics SHALL be deterministic.

#### Scenario: Several errors in one pass
- **WHEN** the template contains three unsupported declarations in different places
- **THEN** the report contains three diagnostics with independent coordinates

#### Scenario: A repeated run gives the same report
- **WHEN** the same template is validated twice
- **THEN** the list of diagnostics matches in content and order

### Requirement: The `a::after` pseudo-element for generated content

The engine SHALL support exactly one pseudo-element — `a::after` — and only for the `content` property. A `content` value SHALL consist of strings, `counter(page)`, `counter(pages)` and `target-counter(attr(href), page)`. Any other selector with a pseudo-element, or any other property inside `a::after`, SHALL be rejected. Outside `a::after` the `content` property is not supported.

#### Scenario: A page number in the table of contents
- **WHEN** the template contains `a.toc::after { content: target-counter(attr(href), page) }`
- **THEN** validation accepts the rule and, after rendering, the target page number is appended to the link text

#### Scenario: Another pseudo-element
- **WHEN** the template contains `p::before { content: "x" }`
- **THEN** validation fails with `css/unsupported-selector`

#### Scenario: A property inside a::after
- **WHEN** the template contains `a::after { color: red }`
- **THEN** validation fails with `css/unsupported-property`

#### Scenario: content outside a::after
- **WHEN** the template contains `p { content: "x" }`
- **THEN** validation fails with `css/unsupported-property`

### Requirement: Profile validation without rendering

The engine SHALL provide a template-check mode that finishes without producing a PDF, with a nonzero exit code when there are errors. This mode SHALL be suitable for running in CI.

#### Scenario: Checking a valid template
- **WHEN** a template that is entirely within the profile is checked
- **THEN** the exit code is zero and no PDF is produced

#### Scenario: Checking a template with an error
- **WHEN** a template with an unsupported property is checked
- **THEN** the exit code is nonzero, the diagnostic is printed and no PDF is produced

### Requirement: A local stylesheet referenced by link

A `link` element with `rel="stylesheet"` and a local `href` SHALL add the stylesheet to the cascade in document order. A `media` attribute whose value is `print`, `all` or empty SHALL apply the sheet; any other value SHALL skip the sheet without a diagnostic.

#### Scenario: Cascade order
- **WHEN** a document declares a `<style>` with a colour, then a `<link>` with another colour, then another `<style>`
- **THEN** the colour from the last declaration applies, as in a normal CSS cascade

#### Scenario: A print-only sheet
- **WHEN** `<link rel="stylesheet" media="print" href="print.css">` sets padding
- **THEN** the padding is applied when the PDF is built

#### Scenario: A screen sheet
- **WHEN** `<link rel="stylesheet" media="screen" href="screen.css">` sets padding
- **THEN** the sheet is skipped, the build succeeds and there are no diagnostics

#### Scenario: An ordinary link
- **WHEN** the document contains `<link rel="canonical" href="https://example.com/">`
- **THEN** the element is ignored and there are no diagnostics

### Requirement: Path boundaries and diagnostics for external styles

Remote style sources and `data:` URLs SHALL be rejected. A missing local file SHALL produce an error naming the path. When a base directory is configured, a permitted path SHALL stay inside it. A diagnostic for a declaration from an external file SHALL name the file, line and column in that file.

#### Scenario: Remote stylesheet
- **WHEN** `<link rel="stylesheet" href="https://example.com/site.css">`
- **THEN** validation fails with `css/link-remote` and no PDF is produced

#### Scenario: Missing file
- **WHEN** `<link rel="stylesheet" href="styles/missing.css">`
- **THEN** the error `css/link-load` is printed with the file path named

#### Scenario: Escaping the base directory
- **WHEN** with a base directory configured, the template links `../../outside.css`
- **THEN** validation fails and the file is not read

#### Scenario: An error inside a linked file
- **WHEN** line 12 of `styles/print.css` declares `display: flex`
- **THEN** the diagnostic points at `styles/print.css:12` with a column, not at the HTML line

### Requirement: External stylesheet encoding

An external stylesheet SHALL be decoded with the document's encoding. A `@charset` rule at the start of the file SHALL be accepted when it names the same encoding, and MUST fail with `css/link-encoding` when it names another. A byte the encoding does not define MUST fail with `css/link-encoding` at that byte. A diagnostic for a rule in the file MUST name its position in the original file.

#### Scenario: A matching charset rule

- **WHEN** the document is windows-1251 and `print.css` starts with `@charset "windows-1251";`
- **THEN** the sheet applies and there are no diagnostics

#### Scenario: A conflicting charset rule

- **WHEN** the document is windows-1251 and `print.css` starts with `@charset "utf-8";`
- **THEN** validation fails with `css/link-encoding` naming both encodings

#### Scenario: An undefined byte in a sheet

- **WHEN** a windows-1251 stylesheet contains byte 0x98, which the encoding does not define
- **THEN** validation fails with `css/link-encoding` at that byte

#### Scenario: A position inside a decoded sheet

- **WHEN** a Cyrillic rule precedes an unsupported declaration in a windows-1251 stylesheet
- **THEN** the diagnostic names the declaration's line and column in the original file, not in the decoded text

### Requirement: Ignoring scripts

A `script` element SHALL be ignored: its content is not executed and takes no part in layout. Validation SHALL emit an `html/ignored-script` warning with the element's position. In strict mode this warning SHALL become an error.

#### Scenario: A script in the template
- **WHEN** the template contains `<script>console.log("x")</script>` and text
- **THEN** the build succeeds, a warning is printed and the script content does not appear in the PDF

#### Scenario: Strict mode
- **WHEN** the build runs with `-strict` and the template contains `<script>`
- **THEN** validation fails and no PDF is produced

#### Scenario: Markup inside a template script
- **WHEN** the script is declared as `<script type="text/template"><p>template</p></script>`
- **THEN** the markup inside is not rendered but also does not raise an error

### Requirement: Stylesheet paths cannot escape through symlinks

When a base directory is configured, the engine SHALL open stylesheets through a root confined to that directory, so a symbolic link inside the base that points outside MUST be rejected. Lexical rejections of `..` and of absolute paths outside the base MUST keep working, and symlinks that stay inside the base MUST be followed.

#### Scenario: A symlink pointing outside
- **WHEN** the base directory contains `link.css` pointing to a file outside it and the document links `link.css`
- **THEN** validation fails with `css/link-path` and the outside file is not read

#### Scenario: A symlink inside the base
- **WHEN** the base directory contains `link.css` pointing to another stylesheet inside it
- **THEN** the stylesheet loads and its rules apply

#### Scenario: Lexical escape still rejected
- **WHEN** the document links `../../outside.css`
- **THEN** validation fails with `css/link-path`

#### Scenario: No base directory configured
- **WHEN** no base directory is configured
- **THEN** relative paths resolve from the working directory as before

### Requirement: Inheritance follows the element chain

The engine SHALL compute styles along the real ancestor chain: `html` → `body`, and `table` → row group (`thead`, `tbody`, `tfoot`) → `tr` → `td`/`th`. Inherited properties set on an ancestor MUST reach its descendants through that chain, and attributes on the `html` element MUST survive parsing so selectors and inline styles on it take effect.

#### Scenario: Font size from html
- **WHEN** the stylesheet declares `html { font-size: 30pt }` and the body contains text without its own size
- **THEN** the text is rendered at 30 pt

#### Scenario: Colour from a table row
- **WHEN** the stylesheet declares `tr { color: red }`
- **THEN** the text in its cells is red

#### Scenario: Inline style on html
- **WHEN** the document declares `<html style="color: red">`
- **THEN** body text inherits the red colour

#### Scenario: Attributes survive parsing
- **WHEN** the `html` element carries a class or id
- **THEN** the attribute is present on the parsed element and selectors can match it

### Requirement: Local assets are opened inside their configured root

Every local file named by the document SHALL be opened inside a configured root:
stylesheets and images inside `BaseDir`, `@font-face` files inside `FontDir` when
set, otherwise `BaseDir`. Absolute paths and symbolic links that leave the root
MUST be rejected with a diagnostic naming the asset. Operator-provided fallback
fonts are trusted paths and are outside this rule. When no root is configured, the
engine reads paths as before.

#### Scenario: Image outside the base directory
- **WHEN** an `<img src>` is an absolute path outside `BaseDir`
- **THEN** conversion fails with a diagnostic naming the path
- **AND** the file is not read

#### Scenario: Image reached through an escaping symlink
- **WHEN** `BaseDir` contains `logo.png` that links to a file outside it
- **THEN** conversion fails with a diagnostic
- **AND** the outside file is not read

#### Scenario: Image inside the base directory
- **WHEN** the document references an image that exists inside `BaseDir`, including through a symlink that stays inside
- **THEN** the image is loaded and embedded

#### Scenario: Font outside the font root
- **WHEN** `@font-face` names a file outside `FontDir`
- **THEN** conversion fails with a `font/unavailable` diagnostic

#### Scenario: Trusted fallback font
- **WHEN** the run passes a fallback font path outside the roots
- **THEN** it is loaded as configured

#### Scenario: No root configured
- **WHEN** neither `BaseDir` nor `FontDir` is set
- **THEN** relative paths resolve from the working directory as before

### Requirement: A file that cannot be read is reported with its reason

A file that exists inside the configured root but cannot be read SHALL be
reported with that reason. The diagnostic MUST NOT claim that the path escapes
the boundary; a boundary escape and a read failure are distinct failures.

#### Scenario: Unreadable file inside the root
- **WHEN** an asset exists inside the root but cannot be read because of permissions
- **THEN** the diagnostic reports the access error
- **AND** it does not claim that the path escapes the boundary

### Requirement: Background image sources are local files

`background-image` SHALL accept a `url()` reference to a local file, resolved against the same configured root as an `<img>` source, or a base64 `data:image/png` / `data:image/jpeg` URI. A remote URL, a `data:` URI of any other type, or an absolute path outside the root SHALL be rejected with a diagnostic naming the source.

#### Scenario: A background image inside the base directory
- **WHEN** the fixture is converted with `BaseDir` set and `background-image: url(cover.jpg)` names a file inside it
- **THEN** the image is loaded from that file and embedded once

#### Scenario: A background image outside the base directory
- **WHEN** `background-image: url(../outside.jpg)` resolves outside `BaseDir`
- **THEN** conversion fails with a diagnostic naming the path, and the file is not read

#### Scenario: A remote background image
- **WHEN** the template sets `background-image: url(https://example.com/cover.jpg)`
- **THEN** conversion fails with a diagnostic naming the source, instead of fetching anything

#### Scenario: A vector data URI
- **WHEN** the template sets `background-image: url("data:image/svg+xml;base64,…")`
- **THEN** conversion fails with a diagnostic naming the image type, because SVG is not decoded

### Requirement: At-rule nesting is depth-limited

The CSS parser SHALL reject a stylesheet whose at-rules are nested deeper than 64 levels with a `resource/limit` diagnostic during parsing, before any recursive walk over the rules. The limit SHALL apply to `@media`, page margin boxes and every other nested at-rule.

#### Scenario: A deeply nested media query is rejected
- **WHEN** a document contains an inline stylesheet with 500,000 nested `@media`
  blocks, below the default input limit
- **THEN** the call returns a `resource/limit` diagnostic
- **AND** the process does not crash

#### Scenario: The limit does not reject valid nesting
- **WHEN** a stylesheet nests at-rules 64 levels deep
- **THEN** validation accepts the stylesheet and the declared rules apply

#### Scenario: One level over the limit
- **WHEN** a stylesheet nests at-rules 65 levels deep
- **THEN** the call returns a `resource/limit` diagnostic naming the nesting limit

### Requirement: Media rules apply at any nesting

Rules inside `@media` blocks whose query matches `print` or `all` SHALL apply to the document, at any nesting depth within the at-rule nesting limit, preserving document order. The same traversal MUST feed the cascade, `@page` resolution and `@font-face` collection; a `@media` with any other query MUST apply nothing.

#### Scenario: Nested print block
- **WHEN** the stylesheet contains `@media print { p { display: none } }`
- **THEN** the paragraph is not rendered

#### Scenario: Nested all block
- **WHEN** the stylesheet contains `@media all { @media print { h1 { color: red } } }`
- **THEN** the `h1` colour is red

#### Scenario: Unsupported media query
- **WHEN** the stylesheet contains `@media screen { p { display: none } }`
- **THEN** validation rejects the query as unsupported

#### Scenario: Page and font rules inside media
- **WHEN** `@page` or `@font-face` is declared inside `@media print`
- **THEN** the page size and the font take effect

### Requirement: White space in preformatted text

With `white-space: pre`, a line feed (LF or CRLF) SHALL start a new line, sequences of spaces MUST be preserved, and no character used for formatting may reach glyph lookup. A tab MUST be treated as a space.

#### Scenario: Two lines
- **WHEN** preformatted text contains `A` and `B` separated by a line feed
- **THEN** two lines are rendered
- **AND** no `text/glyph` error is reported

#### Scenario: Preserved spaces
- **WHEN** preformatted text contains a run of three spaces
- **THEN** the run keeps its width

#### Scenario: Carriage return line feed
- **WHEN** the line ending is CRLF
- **THEN** it produces exactly one line break

### Requirement: Text indent applies to the first line

`text-indent` SHALL shift the first line of its block by the specified length or percentage of the content width. Following lines MUST keep the regular origin, and justification MUST use the reduced width.

#### Scenario: Indented first line
- **WHEN** a paragraph declares `text-indent: 20pt`
- **THEN** its first line starts 20 pt to the right
- **AND** the second line starts at the normal position

#### Scenario: Indent survives an anonymous wrapper
- **WHEN** a block with `text-indent` contains inline content wrapped in an anonymous box
- **THEN** the first line is still indented

### Requirement: Unitless line height is inherited as a factor

A unitless `line-height` SHALL be inherited as a factor and resolved against the font size of the element that uses it. Lengths and percentages MUST keep their absolute semantics.

#### Scenario: Factor follows the child font size
- **WHEN** a parent sets `font-size: 10pt; line-height: 2` and a child sets `font-size: 20pt`
- **THEN** the child's lines are 40 pt apart

#### Scenario: Absolute length stays absolute
- **WHEN** a parent sets `line-height: 30pt` and a child changes its font size
- **THEN** the line height stays 30 pt

### Requirement: A failed asset root stops asset reads

When a configured base or font directory cannot be opened, the conversion SHALL fail
with an `options/asset-root` diagnostic before any asset is read. It MUST NOT continue
with an unbounded root, and a linked path outside the intended directory MUST NOT be
opened.

#### Scenario: A missing base directory

- **WHEN** `BaseDir` names a directory that does not exist and the document links an
  absolute stylesheet outside it
- **THEN** the call returns `options/asset-root`
- **AND** the outside stylesheet is not read or parsed

#### Scenario: A valid base directory

- **WHEN** `BaseDir` exists and the document links a stylesheet inside it
- **THEN** the stylesheet loads and the conversion proceeds

### Requirement: Attributes are validated and hidden hides content

The profile SHALL validate attributes as well as elements and properties: an
attribute outside the documented allow-list MUST be reported with its position, and
the HTML `hidden` attribute MUST remove its subtree from layout. Content marked
`hidden` MUST NOT appear in the PDF.

#### Scenario: A hidden block

- **WHEN** a document contains `<div hidden><p>SECRET</p></div>`
- **THEN** the extracted PDF text does not contain "SECRET"

#### Scenario: An unknown attribute

- **WHEN** an element carries `data-x` or `onclick`
- **THEN** the diagnostic names the attribute and its position

#### Scenario: An allowed attribute

- **WHEN** an element carries a documented attribute such as `colspan` or `href`
- **THEN** no attribute diagnostic is produced

### Requirement: Stylesheet problems are capped and charged (RV4-02)

The CSS tokenizer and parser SHALL charge the work guard for the input they read
and MUST cap the number of stored problems, stopping with a single `resource/limit`
once the cap is reached. A stylesheet MUST NOT be able to produce one diagnostic per
input byte, and the returned diagnostic list MUST stay bounded independently of the
input size.

#### Scenario: A stylesheet of stray braces

- **WHEN** a document contains a `<style>` element of one megabyte consisting of
  `}` characters
- **THEN** the call returns `resource/limit` after a bounded number of diagnostics
- **AND** the memory and the error text stay proportional to the cap, not to the
  input

### Requirement: Adjacent-sibling matching is bounded and charged (RV4-07)

Matching a `+` selector MUST NOT scan every preceding sibling for each element. The
hop SHALL be constant time or charged per sibling visited, so the cost stays within
the documented bound of compounds times ancestor depth.

#### Scenario: Many siblings with an adjacent selector

- **WHEN** a stylesheet contains `p + p` and the document has 80000 sibling
  paragraphs
- **THEN** matching time stays close to the `p p` case
- **AND** a small `MaxOperations` stops the conversion before minutes of matching

### Requirement: A CSS-wide keyword is not a font family (RV4-11)

`font-family` SHALL reject `inherit`, `initial`, `unset` and `revert`, or implement
`inherit`. An identifier that names a CSS-wide keyword MUST NOT be stored as a
family name and MUST NOT fall through to a system fallback font.

#### Scenario: inherit in a font family

- **WHEN** a document declares `p{font-family:inherit}` with a declared family on
  the parent
- **THEN** the paragraph either inherits that family or is rejected with
  `css/unsupported-value`
- **AND** it is never rendered in an undeclared system font

### Requirement: media is a supported attribute of style (RV4-18)

The `media` attribute SHALL be accepted on `<style>` exactly as on `<link>`, and a
value that applies to print MUST apply the sheet. Profile validation MUST NOT report
it as an unsupported attribute, and `Strict` MUST accept a document that uses it.

#### Scenario: A print-only style element

- **WHEN** a document contains `<style media="print">p{color:#00ff00}</style>`
- **THEN** validation reports no `html/unsupported-attribute`
- **AND** the rule applies, including with `Strict: true`

### Requirement: A pseudo-element is only valid in the final compound (RV4-19)

A selector that carries a pseudo-element in any compound other than the last SHALL
be rejected with `css/unsupported-selector`. A pseudo-element MUST NOT match or
style the element selected by a following compound.

#### Scenario: A pseudo-element before a descendant

- **WHEN** a stylesheet contains `a::after span { color:#ff0000 }`
- **THEN** validation rejects the selector
- **AND** the span is not painted red

### Requirement: A percentage height is rejected or resolved (RV4-20)

`height` SHALL either reject a percentage with `css/unsupported-value` or resolve it
against the containing block. A declaration the profile validates MUST NOT be
silently ignored by layout.

#### Scenario: A percentage height

- **WHEN** a document declares `height: 50%` on a block inside a parent with a
  declared height
- **THEN** either the height resolves to half the containing block or validation
  rejects the value
- **AND** the rendered box is never silently treated as `height: auto`

### Requirement: A relative path resolves inside the confined reader's root (RV4-36)

`Root.Read`, `Root.Resolve` and `Root.Size` SHALL interpret a relative path relative
to the configured root directory, not the process working directory, and `Size` MUST
stat through the root handle. A path inside the root MUST NOT be reported as an
escape because the process working directory differs.

#### Scenario: A relative path inside the root

- **WHEN** a root is opened for a directory and a caller reads `sub/file.txt`
  relative to it
- **THEN** the file resolves inside the root
- **AND** the same call from another working directory behaves identically

### Requirement: A nested at-rule the parent cannot represent is an error (RV4-46)

An at-rule nested where its parent cannot represent it SHALL produce a problem
(`css/unsupported-at-rule`) instead of being dropped, and `@page` inside `@page`
MUST be rejected.

#### Scenario: A media rule inside a font face

- **WHEN** a stylesheet contains `@font-face { ... @media print { p{color:red} } }`
- **THEN** validation reports an unsupported nested at-rule
- **AND** the rule is not silently discarded

### Requirement: A compound with two ids cannot match (RV4-47)

A compound that contains more than one id SHALL be rejected, or every id MUST be
required to match and counted in the specificity. A rule such as `#x#y` MUST NOT
match an element that carries only one of the ids.

#### Scenario: Two ids in one compound

- **WHEN** a stylesheet contains `#x#y { color:#ff0000 }` and the document has
  `<p id="y">`
- **THEN** the paragraph is not matched by that rule
- **AND** the rule's specificity is not reduced to a single id

### Requirement: Identifiers decode backslash escapes (RV4-48)

The tokenizer SHALL decode CSS escapes in identifiers and hashes as it already does
in strings, so `caf\e9` names `café` in a family name and in a class selector.

#### Scenario: An escaped family name

- **WHEN** a stylesheet contains `font-family: caf\e9`
- **THEN** the family resolves to `café`
- **AND** the same escape in a selector matches the class `café`

### Requirement: A media query list is parsed as a list (RV4-49)

Validation and stylesheet collection SHALL share one media-list parser.
`@media print, screen` MUST apply for print instead of being rejected, and a list
that contains `print` MUST be treated as applying.

#### Scenario: A media list with print

- **WHEN** a stylesheet contains `@media print, screen { p{color:red} }`
- **THEN** validation accepts the rule
- **AND** the declarations apply to the print layout

### Requirement: A declaration is never dropped silently (RV4-50)

Cascade application MUST NOT discard a declaration that validation accepted without
a diagnostic. Validation SHALL use the resolved parent font size, so a value that
can round to zero at render time is rejected during validation instead.

#### Scenario: A font size that rounds to zero

- **WHEN** a document sets a tiny percentage font size on a child of a one-point
  parent
- **THEN** validation reports `css/unsupported-value` for the declaration
- **AND** the paragraph is not silently rendered at the parent size

### Requirement: always is documented as a break value (RV4-61)

The profile SHALL document `always` as an accepted alias of `page` for
`break-before` and `break-after`, or reject it. The property table in
`docs/profile.md` and the parser MUST agree.

#### Scenario: break-before always

- **WHEN** a template uses `break-before: always`
- **THEN** the parser treats it as `break-before: page`
- **AND** the documented values for the property include `always`

### Requirement: Grammar pieces are shared and their policy is explicit (IMP-12)

Media lists, CSS-wide keywords, identifier escapes and nested at-rules SHALL each
have one implementation and one documented policy, used by validation and by
stylesheet collection. A second parser for any of them MUST NOT exist.

#### Scenario: A new grammar rule (IMP-12)

- **WHEN** a media list, an escape or a nested at-rule is supported
- **THEN** validation and collection use the same parser and agree
- **AND** the policy is stated in `docs/profile.md`

### Requirement: CSS tokenization always progresses and is charged (RV5-02)

Every tokenizer step SHALL consume at least one input byte or stop the parse with a
`css/syntax` problem, so no input can loop. Declaration parsing MUST count toward the
documented work budget and observe cancellation, so a stylesheet MUST NOT spend the
budget one empty token at a time; an exhausted budget or a cancelled context MUST
stop the call promptly with `resource/limit` or the context error.

#### Scenario: A style attribute ends with a lone backslash

- **WHEN** a document contains `<p style="color:red\">x</p>` and `Validate` or
  `Convert` is called
- **THEN** the call returns and reports `css/syntax` for the attribute
- **AND** it does not hang and produces no partial PDF

#### Scenario: A style element ends with a lone backslash

- **WHEN** a document contains `<style>p{color:red\</style>`
- **THEN** the call returns with `css/syntax` or `resource/limit`
- **AND** the tokens it stored stay bounded by the work budget instead of growing
  without limit

#### Scenario: A cancelled context stops declaration parsing

- **WHEN** the caller passes an already cancelled context for a document with a
  style attribute
- **THEN** the call returns `context.Canceled` instead of parsing to completion

#### Scenario: A tiny work budget bounds a stray backslash

- **WHEN** the caller sets a small `MaxOperations` and the document contains a style
  attribute or stylesheet ending in many lone backslashes
- **THEN** the call returns `resource/limit` after a bounded amount of work

#### Scenario: Well-formed CSS is unaffected

- **WHEN** documents and stylesheets use valid escapes such as `content: "\2014"` or
  `font-family: caf\e9`
- **THEN** they parse and render exactly as before
- **AND** the existing CSS tests stay green

### Requirement: Selector escapes decode to data, not to syntax (RV5-26)

A character produced by an escape in a selector SHALL be part of the name it was
written in and MUST NOT be re-read as selector syntax. `.a\.b` MUST be one class
selector matching `class="a.b"`, and `.a\ b` MUST be one class name containing a
space rather than a descendant selector.

#### Scenario: A class name containing a dot

- **WHEN** the stylesheet contains `.a\.b { color:#c00 }` and the document contains
  `<p class="a.b">x</p>`
- **THEN** the rule matches the paragraph and the text is red

#### Scenario: The escaped dot is not a class separator

- **WHEN** the document contains `<p class="a b">x</p>` and no element with the
  single class `a.b`
- **THEN** `.a\.b` does not match that paragraph

#### Scenario: An escaped space

- **WHEN** the stylesheet contains `.a\ b { color:#c00 }`
- **THEN** the selector is one compound whose class name is `a b`
- **AND** it is not parsed as a descendant selector

#### Scenario: A plain compound is unchanged

- **WHEN** the stylesheet contains `.a.b { color:#c00 }`
- **THEN** it still requires both classes `a` and `b` on one element

### Requirement: url() decodes escapes and honours an escaped parenthesis (RV5-27)

A `url()` argument SHALL have its CSS escapes decoded whether it is quoted or
unquoted, and an unquoted `\)` MUST close the argument. `url("a\20 b.png")` MUST name
the same file as `url("a b.png")`, and a decoded escape MUST NOT reach the asset
loader as part of a literal path.

#### Scenario: An escaped space in a quoted url

- **WHEN** the stylesheet contains `background-image: url("a\20 b.png")` and the
  directory holds a file named `a b.png`
- **THEN** the image loads and is painted
- **AND** the conversion does not report `img/load`

#### Scenario: An escaped closing parenthesis, unquoted

- **WHEN** the value is `background-image: url(a\)b.png)` and the directory holds a
  file named `a)b.png`
- **THEN** the argument is `a)b.png` and the image loads

#### Scenario: Other sources are still rejected

- **WHEN** the value is a remote URL or a `data:` URI the profile does not accept
- **THEN** validation still reports the existing diagnostic
- **AND** the escape handling does not widen the accepted sources

#### Scenario: A plain url is unchanged

- **WHEN** the value is `url(cover.jpg)`
- **THEN** it resolves exactly as before

### Requirement: A style attribute is a declaration list, not a stylesheet (RV5-06)

A `style` attribute SHALL be parsed as a declaration list that ends only at end of
input. A `{` or `}` in it MUST produce `css/syntax`, and an at-rule MUST produce
`css/unsupported-at-rule`; neither may end the list silently or be discarded.
Declarations before the offending token SHALL stay parsed, validated and available
to the cascade, and every declaration MUST be either kept or reported.

#### Scenario: A leading brace before an unsupported declaration

- **WHEN** a document contains `<p style="} display:flex">x</p>`
- **THEN** validation reports `css/syntax` for the `}`
- **AND** it also reports `css/unsupported-value` for `display:flex`
- **AND** the call produces no PDF

#### Scenario: A brace in the middle of the attribute

- **WHEN** the attribute is `style="color:red} display:flex"`
- **THEN** validation reports `css/syntax` for the `}`
- **AND** `color:red` remains in the declaration list available to the cascade
- **AND** `display:flex` is reported instead of being dropped

#### Scenario: A stray opening brace

- **WHEN** the attribute is `style="color:red{display:flex"`
- **THEN** validation reports `css/syntax` for the `{`
- **AND** `display:flex` is reported as an unsupported value

#### Scenario: A media rule inside a style attribute

- **WHEN** the attribute is `style="@media print { p { display: flex } }"`
- **THEN** validation reports `css/unsupported-at-rule`
- **AND** the inner `display:flex` is not silently accepted

#### Scenario: A page rule inside a style attribute

- **WHEN** the attribute is `style="@page { size: A4 }"`
- **THEN** validation reports `css/unsupported-at-rule`
- **AND** no page setup is taken from the attribute

#### Scenario: A well-formed style attribute is unchanged

- **WHEN** the attribute is `style="color:#c00;font-size:10pt"`
- **THEN** validation reports nothing
- **AND** both declarations are applied as before
