# Spec Delta

## ADDED Requirements

### Requirement: A spanning cell is covered by its rows

The sum of the heights of the rows a table cell spans SHALL be at least the cell's outer height. When the covered rows would be shorter, layout MUST grow them by the deficit, so the cell's content stays inside the cell and the block that follows the table starts at the table's bottom edge, never above the cell's content.

#### Scenario: Tall spanning cell over short rows
- **WHEN** a first-row cell with `rowspan="2"` contains more lines than the two rows would otherwise need, and a paragraph follows the table
- **THEN** the paragraph starts below the last line of the spanning cell
- **AND** conversion succeeds with no overflow diagnostic

#### Scenario: Empty covered row
- **WHEN** the second row covered by a `rowspan` cell has no cells of its own
- **THEN** the covered rows still have the height the cell needs

#### Scenario: Measurement agrees with placement
- **WHEN** the table is measured for a `break-inside: avoid` decision or for a page-fit decision
- **THEN** the measured height equals the height used when the table is placed

#### Scenario: Ordinary spans keep their geometry
- **WHEN** a spanning cell already fits inside the rows it covers
- **THEN** the row heights and the page count are unchanged
