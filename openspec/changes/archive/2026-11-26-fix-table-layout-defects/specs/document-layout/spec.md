# Spec Delta

## ADDED Requirements

### Requirement: Scaled table columns fit after a 64-bit sum (RV5-16)

The running sum of a table's column widths SHALL be computed in 64 bits, and the
scaled widths MUST be clamped so the last column's right edge stays inside the
content box. When declared widths exceed the content box, the single
`layout/overflow` warning MUST be emitted only after that clamp and MUST describe
the geometry the conversion produced. No cell box may extend past the page without
a diagnostic.

#### Scenario: Two very wide columns

- **WHEN** a table declares two cells of `width:2000000pt` on A4
- **THEN** every cell box's right edge is at most the content box's right edge
- **AND** no cell is painted outside the page
- **AND** the emitted warning, if any, states that the columns were scaled

#### Scenario: A table that fits

- **WHEN** the declared column widths fit the content box
- **THEN** the widths and positions are unchanged
- **AND** no `layout/overflow` is reported

### Requirement: An unbreakable word in a table cell is reported (RV5-17)

A word wider than a table cell's content width that offers no break opportunity
SHALL report `layout/overflow` once for the cell, exactly as the same word in a
block does. The word MUST NOT be painted past the page edge without a diagnostic,
and a word that fits MUST NOT be reported.

#### Scenario: A long word inside a cell

- **WHEN** a cell contains a thousand-character word with no spaces
- **THEN** the conversion reports `layout/overflow` for the cell
- **AND** the diagnostic counts the clipping once

#### Scenario: A short word inside a cell

- **WHEN** a cell's words fit its content width
- **THEN** no overflow diagnostic is reported
