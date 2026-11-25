# Spec Delta

## MODIFIED Requirements

### Requirement: Supported units of measurement

The engine SHALL support the units `px`, `pt`, `mm`, `cm`, `in`, `%` and `em`.
Percentages SHALL resolve against the containing block width, and `em` against the
element's computed `font-size`. The units `rem`, `vw`, `vh` and `ch` SHALL be
rejected as unsupported.

#### Scenario: Mixed units

- **WHEN** the template sets `width: 100mm` and `padding: 1.5em` with
  `font-size: 12pt`
- **THEN** the widths are converted to a single internal unit without loss of
  precision
- **AND** the block width equals 100 mm

#### Scenario: Unsupported unit

- **WHEN** the template contains `margin: 2rem`
- **THEN** validation fails with the `rem` unit named

#### Scenario: A percentage width with a percentage margin

- **WHEN** a block declares `width: 50%; margin-left: 10%` in a containing block of
  481,890 millipoints
- **THEN** the border box is 240,945 wide and starts 48,189 from the containing
  block's left edge

#### Scenario: Percentage padding and bottom margin

- **WHEN** a box declares `padding: 0 10%` or `margin-bottom: 10%`
- **THEN** both resolve against the containing block width, not the box's own width

#### Scenario: Percentage text indent

- **WHEN** a block with a 100 pt content width sits in a 481,890 millipoint
  containing block and declares `text-indent: 10%`
- **THEN** the first line starts 48,189 millipoints in, not 10,000
