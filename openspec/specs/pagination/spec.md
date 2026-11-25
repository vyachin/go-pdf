# pagination Specification

## Purpose

Defines how a continuous flow of boxes is cut into pages: page size and margins, headers and footers, numbering, break priorities, and two-pass resolution of page numbers for a table of contents.

## Requirements

### Requirement: Page size and margins

The engine SHALL determine the page size from `@page { size }` and, when absent, from the run options. The names `A4`, `A5`, `Letter`, explicit lengths, and the `portrait` or `landscape` orientation — alone or combined with a name — MUST be supported. An orientation without a size name applies to the size from the run options. Margins declared in `@page` MUST shrink the content area and MUST NOT overlap the headers and footers.

#### Scenario: Size from CSS
- **WHEN** the template declares `@page { size: A4 }` and the run specifies A5
- **THEN** the PDF is produced as A4

#### Scenario: Default size
- **WHEN** the template does not declare `@page size` and the run specifies A5
- **THEN** the PDF is produced as A5

#### Scenario: Landscape named size
- **WHEN** the template declares `@page { size: A4 landscape }`
- **THEN** the page is 297 mm wide and 210 mm high
- **AND** headers, footers and margins follow the new sides

#### Scenario: Portrait orientation
- **WHEN** the template declares `@page { size: A4 portrait }`
- **THEN** the page is 210 mm wide and 297 mm high

#### Scenario: Orientation without a size name
- **WHEN** the template declares `@page { size: landscape }` and the run specifies A5
- **THEN** the page is 210 mm wide and 148 mm high

#### Scenario: Orientation together with explicit lengths
- **WHEN** the template declares `@page { size: 210mm 297mm landscape }`
- **THEN** validation fails because the CSS grammar does not allow an orientation together with lengths

#### Scenario: Unknown word in the size
- **WHEN** the template declares `@page { size: A4 sideways }`
- **THEN** validation fails and names the unsupported value

### Requirement: Page margins are lengths and orientation flips a fallback

Margins declared in `@page` SHALL be non-negative lengths: `auto`, percentages and
negative values MUST be rejected with `css/unsupported-value`. An `em` margin SHALL
resolve against the initial font size, not the page width. `size: portrait` MUST swap
a landscape fallback size, and `size: landscape` MUST swap a portrait one.

#### Scenario: An em page margin

- **WHEN** `@page { size: A4; margin: 1em }` is converted
- **THEN** every margin is 12 pt, not 595 pt

#### Scenario: An auto page margin

- **WHEN** a template declares `@page { margin: auto }`
- **THEN** validation fails with `css/unsupported-value`

#### Scenario: A negative page margin

- **WHEN** a template declares `@page { margin: -20mm }`
- **THEN** validation fails with `css/unsupported-value`

#### Scenario: Portrait with a landscape fallback

- **WHEN** `Options.PageSize` is `300mmx200mm` and the document declares
  `@page { size: portrait }`
- **THEN** the page is 200 mm wide and 300 mm high

### Requirement: Headers and footers in margin boxes

The engine SHALL support the margin boxes `@top-left`, `@top-center`, `@top-right`, `@bottom-left`, `@bottom-center` and `@bottom-right` with content made of strings and counters. Margin box content MUST be repeated on every page, including the first and the last.

#### Scenario: Footer on every page
- **WHEN** the template declares `@bottom-center { content: "Стр. " counter(page) " из " counter(pages) }`
- **THEN** every page carries that footer with correct numbers

#### Scenario: Empty footer
- **WHEN** a margin box is not declared
- **THEN** the corresponding area stays empty and does not affect the content area

### Requirement: Page counters

The engine SHALL support `counter(page)` and `counter(pages)` inside margin boxes, returning the current page number and the total page count. The values MUST be consistent across the document: no page may get a number larger than the total.

#### Scenario: Numbering from the first page
- **WHEN** the document occupies three pages
- **THEN** every page footer contains "из 3" and the numbers run 1, 2, 3

#### Scenario: Total consistency
- **WHEN** the document is rendered and the total page count is known
- **THEN** no page reports a `counter(pages)` value that differs from the actual count

### Requirement: Page break priorities

The engine SHALL support `break-before`, `break-after` and `break-inside` with the values `auto`, `page` and `avoid`, as well as the legacy `page-break-*` properties. When an avoided break conflicts with a forced one, the forced `page` break MUST win. A pending forced break SHALL be taken only when the next box paints content; an empty box MUST NOT consume it and MUST NOT create a page.

#### Scenario: Avoid a break inside a block
- **WHEN** a block with `break-inside: avoid` does not fit into the rest of the page
- **THEN** the whole block moves to the next page, leaving empty space behind

#### Scenario: Conflict between avoid and a forced break
- **WHEN** a descendant of an element with `break-inside: avoid` requests
  `break-before: page`
- **THEN** the break happens and the element continues on the next page

#### Scenario: An empty box after a forced break
- **WHEN** `break-after: page` is followed by an empty `<div>` or a box whose only
  visible content is a background or height
- **THEN** the break is still taken before that box
- **AND** the box and the following paragraph start on the next page

#### Scenario: A break before an empty box
- **WHEN** an empty box declares `break-before: page` and no visible content follows
- **THEN** no blank page is produced

### Requirement: Adjacent forced breaks collapse into one

When a `break-after: page` and a `break-before: page` meet at the same boundary, the engine SHALL make exactly one page transition, for blocks and table rows alike. A pending forced break MUST be treated as satisfied by any page transition already taken at that boundary, and the engine MUST NOT emit a page with no body content between the two elements.

#### Scenario: Break after followed by break before
- **WHEN** a paragraph has `break-after: page` and the next paragraph has
  `break-before: page`
- **THEN** the document has exactly two pages
- **AND** the second paragraph is on the second page

#### Scenario: Pending break satisfied by another transition
- **WHEN** a block with `break-after: page` is followed by a block that moves to the
  next page for another reason
- **THEN** no extra empty page is created

#### Scenario: Table rows with a repeated header
- **WHEN** a row with `break-after: page` is followed by a row with
  `break-before: page` and the table repeats a header
- **THEN** exactly one page transition happens at that boundary
- **AND** no page contains only repeated headers

#### Scenario: Trailing forced break
- **WHEN** the last block in the document has `break-after: page`
- **THEN** no empty page is appended

### Requirement: Row-level breaks preserve ancestor fragments

A forced break declared on a table row SHALL be handled by the page loop, not by creating a page inside table placement. Before the new page, every enclosing fragment (box background, border and table background) MUST be completed and reset, so no operation is painted after the content it should sit behind or with a previous page's coordinates.

#### Scenario: A table background across a row break
- **WHEN** a table with a background colour and a row with `break-after: page`
  continues on a second page
- **THEN** page 1 paints the table background before the row content
- **AND** page 2 paints it with page 2's coordinates

#### Scenario: An ancestor background survives
- **WHEN** `<body>` has a background and a row forces a page break
- **THEN** both pages paint the background across their full content area

### Requirement: Orphans and widows

The engine SHALL honour `orphans` and `widows`, with defaults of 2 and 2. If a break would leave fewer lines than allowed, the break MUST shift so that the condition holds.

#### Scenario: A lonely first line
- **WHEN** only one line of a paragraph fits on a page and `orphans` is 2
- **THEN** the paragraph starts on the next page

#### Scenario: A lonely last line
- **WHEN** a break leaves one paragraph line on the new page and `widows` is 2
- **THEN** the break shifts so that at least two lines move to the new page

### Requirement: Repeated table header

The engine SHALL repeat `thead` content at the top of every page on which a table continues, and MUST render `tfoot` only on the last page of the table. A table row with `break-inside: avoid` MUST NOT be split between pages. Every row of a multi-row `thead` MUST appear exactly once on the first page.

#### Scenario: A table across three pages
- **WHEN** a table occupies three pages and has a `thead`
- **THEN** each of the three pages starts with the repeated header

#### Scenario: A row is not split
- **WHEN** a table row declared with `break-inside: avoid` does not fit into the rest of the page
- **THEN** the whole row moves to the next page

#### Scenario: Two header rows
- **WHEN** the `thead` contains two rows and the table continues on a second page
- **THEN** the first page shows both header rows once
- **AND** the second page starts with both header rows repeated once

### Requirement: Element taller than the page

If a block does not fit on an empty page, the engine SHALL place it with overflow and emit a diagnostic that the content extends beyond the page. The engine MUST NOT loop on such an element.

#### Scenario: A table taller than the page
- **WHEN** a single table row is taller than the content area
- **THEN** rendering finishes, the content is clipped at the page boundary and a diagnostic is emitted

### Requirement: Pagination stops when no progress is possible

When a page cannot accept any content, the engine SHALL stop paginating and report an error diagnostic. It MUST NOT return a successful document that lost content or consists of empty pages. A safety iteration limit MAY remain, but reaching it MUST be an error rather than a warning.

#### Scenario: Line taller than the page
- **WHEN** a template sets `line-height` larger than the content area of a page
- **THEN** conversion fails with a `layout/no-progress` diagnostic
- **AND** no PDF is written

#### Scenario: Empty output is never successful
- **WHEN** layout cannot place any content on a fresh page
- **THEN** the result contains an error diagnostic
- **AND** the conversion returns an error

#### Scenario: Content that fits keeps working
- **WHEN** a normal multi-page document is converted
- **THEN** conversion succeeds and the page count matches the content

### Requirement: Table of contents by page numbers

The engine SHALL resolve `target-counter(attr(href), page)` in the `content` property of the `a::after` pseudo-element and MUST re-render until the values converge, so that the numbers in the table of contents match the actual pages of their targets. Convergence MUST be reached within a bounded number of passes, after which a diagnostic is emitted.

#### Scenario: Page numbers in the table of contents
- **WHEN** an anchor `<a class="toc" href="#sec1">` with the rule `a.toc::after { content: target-counter(attr(href), page) }` points to a section that starts on page 4
- **THEN** the table of contents shows 4, matching the actual page of that section

#### Scenario: Link to a missing anchor
- **WHEN** `href` points to an identifier that does not exist in the document
- **THEN** a diagnostic about the unknown target is emitted and the document is still produced

#### Scenario: The table of contents changes pagination
- **WHEN** substituting page numbers grows the table of contents and shifts the sections
- **THEN** the engine performs another pass and emits consistent numbers

#### Scenario: Convergence not reached
- **WHEN** the numbers do not stabilise within the allowed number of passes
- **THEN** a diagnostic about failed convergence is emitted with the number of passes

### Requirement: Margin boxes report shaping failures

When the text of a margin box cannot be shaped because a character is missing from the font, the engine SHALL emit an error diagnostic `text/glyph` naming the margin box and MUST NOT produce a document that silently omits the header or footer.

#### Scenario: Missing glyph in a footer
- **WHEN** `@bottom-center` contains a character absent from the selected font
- **THEN** conversion fails with a `text/glyph` diagnostic naming the box

#### Scenario: Ordinary footer
- **WHEN** the footer text uses characters the font has
- **THEN** it is rendered on every page as before

### Requirement: Block images are placed as replaced blocks

An `img` with `display: block` SHALL be placed as a replaced block: resolved with the same sizing rules as an inline image, painted at the content-box origin, and advanced in the flow like any other block.

#### Scenario: Image between paragraphs
- **WHEN** the document contains a paragraph, a block image and another paragraph
- **THEN** the page contains an image operation
- **AND** both paragraphs are rendered around it

#### Scenario: Image only document
- **WHEN** the body consists of one block image
- **THEN** it is painted once

### Requirement: A box that can never fit does not start a page (RV4-12)

A `break-inside: avoid` box taller than the content height SHALL NOT force a page
transition. The transition MUST be taken only when the box can fit, and a refused
transition MUST stop placement rather than repeat the attempt on the new page.

#### Scenario: An avoid box taller than the page

- **WHEN** a document contains a `break-inside: avoid` block of sixty lines, taller
  than the content height, followed by a paragraph
- **THEN** no empty page precedes the block
- **AND** the block is split across the pages it needs, followed by the paragraph

### Requirement: A spanning cell that crosses a page break is fragmented (RV4-13)

A `rowspan` cell that crosses a page break SHALL paint only the part inside the
current page and continue on the next page. Its background, border and content MUST
NOT be painted outside the page, and a continuation fragment MUST appear on the
following page.

#### Scenario: A rowspan taller than the page

- **WHEN** a table has a `rowspan=60` cell that covers more than one page
- **THEN** the cell's background and border are clipped to the page
- **AND** the continuation is painted on the next page
- **AND** the cell's text stays inside the page

### Requirement: A declared height is reserved once across fragments (RV4-14)

A declared `height` SHALL be consumed across the fragments of a box, not reserved in
full on each fragment, so the box occupies its declared height once.

#### Scenario: A tall box that splits

- **WHEN** a box with `height: 400pt` and more content than fits on one page is
  split across two pages
- **THEN** it reserves 400 pt in total, not 400 pt per fragment
- **AND** the following paragraph sits at the distance the declared height implies
