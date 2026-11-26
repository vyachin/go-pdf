# Spec Delta

## ADDED Requirements

### Requirement: Measurement and placement saturate before storing (RV5-05)

Every geometry sum used to measure, place or paint a box SHALL be computed in 64
bits and saturated before it is stored in `int32`. A measured height MUST never be
negative. A box whose geometry exceeds the page MUST report `layout/overflow`
exactly once and MUST NOT drop content silently.

#### Scenario: Huge padding and height

- **WHEN** a block declares `padding:2000000pt` and `height:2000000pt`
- **THEN** its measured outer height is non-negative and clamped
- **AND** the conversion reports `layout/overflow` once for the box
- **AND** no coordinate wraps to the opposite side of the page

#### Scenario: A cell with a huge line height

- **WHEN** a table cell declares `line-height:2000000pt` and contains the lines
  `A` and `B`
- **THEN** both lines are painted, or the clipping is reported with
  `layout/overflow`
- **AND** the block that follows the table starts below the cell, never above it

#### Scenario: Huge margins

- **WHEN** a document declares padding or a page margin of 2,000,000 pt
- **THEN** the content box is clamped to a non-negative size
- **AND** no painted X or Y coordinate is negative without a diagnostic
