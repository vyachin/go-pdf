# Spec Delta

## ADDED Requirements

### Requirement: An empty table does not count as page content (RV5-18)

A table SHALL count as page content only when it can paint something: a non-empty
row or cell, or its own non-zero height, background or visible border. An empty
table MUST NOT consume a pending forced break, MUST NOT count as page body, and
MUST NOT create a blank page. A table that paints only a height or a background
MUST still consume the break.

#### Scenario: An empty table with a forced break

- **WHEN** a paragraph is followed by
  `<table style="break-before:page"><tr><td></td></tr></table>`
- **THEN** the document has one page
- **AND** no empty page is appended

#### Scenario: A height-only table after a break

- **WHEN** a paragraph with `break-after:page` is followed by a table that paints
  only `height:50pt;background:#f00`
- **THEN** the table is painted on the next page
- **AND** the paragraph after it is on the same page as the table

#### Scenario: A table with text rows

- **WHEN** a table has a row with visible cell text
- **THEN** it counts as page content and consumes a pending break as before
