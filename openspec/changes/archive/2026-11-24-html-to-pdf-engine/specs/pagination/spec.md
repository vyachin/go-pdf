# Spec Delta

## Purpose

Defines how a continuous flow of boxes is cut into pages: page dimensions and margins, page headers and footers, numbering, break priorities and two-pass resolution of page numbers for the table of contents.

## ADDED Requirements

### Requirement: Page size and margins

The engine SHALL determine the page size from `@page { size }` and, when it is absent, from the run parameter. The values `A4`, `A5`, `Letter` as well as explicit dimensions in length units MUST be supported. Margins set in `@page` MUST shrink the content area and MUST NOT overlap the page headers and footers.

#### Scenario: Size from CSS
- **WHEN** the template declares `@page { size: A4 }` and the run sets A5
- **THEN** the PDF is created in A4 format

#### Scenario: Default size
- **WHEN** the template does not declare `@page size` and the run sets A5
- **THEN** the PDF is created in A5 format

### Requirement: Page headers and footers in margin boxes

The engine SHALL support the margin boxes `@top-left`, `@top-center`, `@top-right`, `@bottom-left`, `@bottom-center`, `@bottom-right` with content from strings and counters. The content of a margin box MUST repeat on every page, including the first and the last.

#### Scenario: A footer on every page
- **WHEN** the template sets `@bottom-center { content: "Стр. " counter(page) " из " counter(pages) }`
- **THEN** every page of the document contains this footer with the correct numbers

#### Scenario: An empty footer
- **WHEN** a margin box is not declared
- **THEN** the corresponding area stays empty and does not affect the content area

### Requirement: Page counters

The engine SHALL support `counter(page)` and `counter(pages)` inside margin boxes, returning the number of the current page and the total number of pages. The values MUST be consistent throughout the document: no page may get a number greater than the total number of pages.

#### Scenario: Numbering from the first page
- **WHEN** the document spans three pages
- **THEN** the footers contain "из 3" on every page, and the numbers run 1, 2, 3

#### Scenario: Consistency of the total
- **WHEN** the document has been rendered and the total number of pages is known
- **THEN** on no page does `counter(pages)` diverge from the actual count

### Requirement: Page break priorities

The engine SHALL support `break-before`, `break-after` and `break-inside` with the values `auto`, `page` and `avoid`, as well as the legacy `page-break-*`. When a prohibition of a break conflicts with the need to break, the forced break `page` MUST win.

#### Scenario: Prohibition of a break inside a block
- **WHEN** a block with `break-inside: avoid` does not fit in the remainder of the page
- **THEN** the block moves to the next page as a whole, leaving empty space

#### Scenario: Conflict of a prohibition and a forced break
- **WHEN** inside an element with `break-inside: avoid` a descendant requires `break-before: page`
- **THEN** the break happens, and the element continues on the next page

### Requirement: Orphans and widows

The engine SHALL honour `orphans` and `widows`; the default values are 2 and 2. If a break would leave fewer lines than allowed, the break MUST shift so that the condition holds.

#### Scenario: A lone first line
- **WHEN** only one line of a paragraph fits on the page and `orphans` equals 2
- **THEN** the paragraph starts on the next page

#### Scenario: A lone last line
- **WHEN** after a break one line of the paragraph is left on the new page and `widows` equals 2
- **THEN** the break shifts so that at least two lines move to the new page

### Requirement: Repetition of the table header

The engine SHALL repeat the content of `thead` at the start of every page the table continues onto, and SHALL output `tfoot` only on the last page of the table. A table row with `break-inside: avoid` must not break between pages.

#### Scenario: A table over three pages
- **WHEN** a table spans three pages and has a `thead`
- **THEN** each of the three pages starts with the repeated header

#### Scenario: A row does not break
- **WHEN** a table row is declared with `break-inside: avoid` and does not fit in the remainder of the page
- **THEN** the row moves to the next page as a whole

### Requirement: An element taller than the page

If a block does not fit on an empty page, the engine SHALL place it with overflow and emit a diagnostic that the content goes beyond the page boundaries. The engine MUST NOT loop on such an element.

#### Scenario: A table taller than the page
- **WHEN** one table row is taller than the height of the content area
- **THEN** rendering finishes, the content is clipped at the page boundary, a diagnostic is emitted

### Requirement: Table of contents by page numbers

The engine SHALL resolve `target-counter(attr(href), page)` in the `content` property of the `a::after` pseudo-element and SHALL perform a repeated render for it until convergence: the numbers in the table of contents match the actual pages of the targets. Convergence MUST be reached in no more than the agreed number of passes, after which a diagnostic is emitted.

#### Scenario: Page numbers in the table of contents
- **WHEN** the link `<a class="toc" href="#sec1">` with the rule `a.toc::after { content: target-counter(attr(href), page) }` points to a section that starts on page 4
- **THEN** the table of contents shows 4, and it matches the actual page of the section

#### Scenario: A link to a non-existent anchor
- **WHEN** the `href` references an identifier that does not exist in the document
- **THEN** a diagnostic about an unknown target is emitted, and the document is still created

#### Scenario: The table of contents changes the pagination
- **WHEN** after the numbers are substituted the table of contents grew and shifted the sections
- **THEN** the engine performs a repeated pass and produces consistent numbers

#### Scenario: Convergence not reached
- **WHEN** the numbers did not stabilise within the allotted number of passes
- **THEN** a diagnostic about the unreached convergence is emitted, naming the number of passes
