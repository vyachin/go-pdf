# Spec Delta

## ADDED Requirements

### Requirement: Extraction follows the page tree

The test tooling SHALL derive pages and content streams from the PDF page tree and the
`/Contents` reference of each page, not by scanning every inflated stream for drawing
operators. The order of pages and streams MUST be deterministic and MUST match the page
order.

#### Scenario: A font that contains TJ bytes

- **WHEN** a document embeds an image or font whose bytes contain `TJ`
- **THEN** the extraction reports the same page count as the page tree

#### Scenario: Stable order

- **WHEN** the same document is extracted 100 times
- **THEN** the content-stream order is identical every time

#### Scenario: Page indices match the tree

- **WHEN** a test reads page 2 by index
- **THEN** it gets the second page of the page tree
