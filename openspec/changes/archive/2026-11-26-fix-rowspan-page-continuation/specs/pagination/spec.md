# Spec Delta

## ADDED Requirements

### Requirement: Every spanning cell is continued across page breaks (RV5-03)

Every `rowspan` cell that crosses a page break SHALL continue on the next page,
including when two spanning cells share the same row. Its text, background and all
four border sides MUST be painted on every page the span covers, clipped to that
page. If a fragment cannot be produced, the conversion MUST report `layout/overflow`
once for the cell instead of dropping content.

#### Scenario: A 100-line span over three pages

- **WHEN** a first-row cell with `rowspan="100"` contains one hundred text lines on
  A4
- **THEN** the document has three pages and all one hundred lines are extractable
- **AND** every page the span covers carries the cell's background and four border
  sides
- **AND** no line is dropped and no diagnostic is reported

#### Scenario: Two spanning cells in one row

- **WHEN** a row contains two cells whose spans both cross the same page break
- **THEN** both cells continue on the next page
- **AND** the second cell does not overwrite the first cell's continuation

#### Scenario: A span ends inside a page

- **WHEN** the rows covered by a spanning cell end in the middle of a page
- **THEN** the last fragment stops at the end of the last covered row
- **AND** it does not extend over the rows below the span

#### Scenario: A fragment cannot be produced

- **WHEN** the remaining content of a spanning cell cannot be placed on a page
- **THEN** the conversion reports exactly one `layout/overflow` naming the cell
- **AND** the rest of the document is still rendered
