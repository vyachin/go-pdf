# Spec Delta

## MODIFIED Requirements

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
