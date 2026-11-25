# Spec Delta

## ADDED Requirements

### Requirement: Table measurement uses the built grid

A table's measurement SHALL be computed from the grid that layout builds. A column
width, a cell width or a row height derived from an unbuilt grid MUST NOT be used for
a `break-inside: avoid` decision, so the measured table height equals the height it
occupies.

#### Scenario: A multi-column table with an avoided break

- **WHEN** a two-column table with automatic widths and `break-inside: avoid` fits on
  the next page but not on the rest of the current one
- **THEN** the measured height matches the placed height
- **AND** the whole table moves to the next page

### Requirement: A line's natural width includes its indent and images

The natural width of a line SHALL include the first-line indent and every inline
image placed on it, even when the image precedes the first word. Alignment and the
wrap decision MUST use that width, so a right-aligned line with an indent and a
leading image ends at the content edge.

#### Scenario: An indented line with a leading image

- **WHEN** a right-aligned paragraph declares `text-indent: 20pt` and starts with a
  20 pt inline image
- **THEN** the line's right edge equals the content-box right edge
- **AND** the first word does not overflow the box
