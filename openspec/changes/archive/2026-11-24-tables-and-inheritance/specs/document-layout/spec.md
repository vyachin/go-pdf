# Spec Delta

## ADDED Requirements

### Requirement: Rowspan shifts following cells

The layout SHALL place every table cell in the grid column it occupies. A cell in a following row MUST start after columns taken by cells that continue from earlier rows, so its horizontal position aligns with the cells of the same column above it.

#### Scenario: A cell under a spanning cell
- **WHEN** the first row has a `rowspan="2"` cell followed by another cell, and the second row has one cell
- **THEN** the second row's cell starts in the column of the first row's second cell
- **AND** it does not sit beneath the spanning cell

#### Scenario: Colspan combined with rowspan
- **WHEN** a spanning cell covers two columns and two rows
- **THEN** cells in the next two rows start after those two columns

#### Scenario: Table without spans
- **WHEN** a table has no `colspan` or `rowspan`
- **THEN** cells keep their sequential positions and widths
