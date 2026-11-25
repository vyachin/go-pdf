# Spec Delta

## ADDED Requirements

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
