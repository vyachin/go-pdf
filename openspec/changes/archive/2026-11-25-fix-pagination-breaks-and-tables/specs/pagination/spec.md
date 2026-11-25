# Spec Delta

## MODIFIED Requirements

### Requirement: Page break priorities

The engine SHALL support `break-before`, `break-after` and `break-inside` with the
values `auto`, `page` and `avoid`, as well as the legacy `page-break-*` properties.
When an avoided break conflicts with a forced one, the forced `page` break MUST win.
A pending forced break SHALL be taken only when the next box paints content; an empty
box MUST NOT consume it and MUST NOT create a page.

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

## ADDED Requirements

### Requirement: Adjacent forced breaks collapse into one

When a `break-after: page` and a `break-before: page` meet at the same boundary, the
engine SHALL make exactly one page transition, for blocks and table rows alike. A
pending forced break MUST be treated as satisfied by any page transition already taken
at that boundary, and the engine MUST NOT emit a page with no body content between the
two elements.

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

A forced break declared on a table row SHALL be handled by the page loop, not by
creating a page inside table placement. Before the new page, every enclosing fragment
(box background, border and table background) MUST be completed and reset, so no
operation is painted after the content it should sit behind or with a previous page's
coordinates.

#### Scenario: A table background across a row break

- **WHEN** a table with a background colour and a row with `break-after: page`
  continues on a second page
- **THEN** page 1 paints the table background before the row content
- **AND** page 2 paints it with page 2's coordinates

#### Scenario: An ancestor background survives

- **WHEN** `<body>` has a background and a row forces a page break
- **THEN** both pages paint the background across their full content area
