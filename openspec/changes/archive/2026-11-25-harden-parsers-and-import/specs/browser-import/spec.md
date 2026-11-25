# Spec Delta

## MODIFIED Requirements

### Requirement: Layout constructs are lowered to the profile

The import SHALL lower a single-line flex column to block layout, a box-covering
absolutely positioned image to a background image, and `box-sizing: border-box` with
an explicit dimension to the content-box model. A flex item's vertical margins SHALL
become padding on a generated wrapper, so the flex spacing survives without margin
collapsing. The positioned ancestor of the lowered background image MUST have an
explicit height.

#### Scenario: A flex column

- **WHEN** a container is a flex column with centred items and margins
- **THEN** it becomes a block whose item wrappers keep the same vertical spacing

#### Scenario: A full-page background image

- **WHEN** an absolutely positioned image covers a positioned ancestor with an
  explicit height
- **THEN** the image becomes the ancestor's cover background and the `<img>` element
  is removed

#### Scenario: Border-box sizing

- **WHEN** an element declares `box-sizing: border-box`, an explicit height and
  bottom padding
- **THEN** the padding is subtracted from the content height and the border box keeps
  its declared size

## ADDED Requirements

### Requirement: Lowering uses resolved border and margin values

The import SHALL resolve an `em` border against the element's `font-size` before
subtracting it from a border-box dimension. When a flex item's margin cannot be
represented as profile padding (`auto` or a negative value), the import MUST report a
diagnostic instead of dropping it silently.

#### Scenario: An em border with border-box

- **WHEN** a 500 pt border-box declares `border: 1em solid` and `font-size: 12pt`
- **THEN** the imported content size is 476 pt, not 300 pt

#### Scenario: An auto flex margin

- **WHEN** a flex item declares `margin: 10pt auto`
- **THEN** the vertical 10 pt is lowered to wrapper padding
- **AND** a warning reports that the horizontal `auto` cannot be represented

#### Scenario: A negative flex margin

- **WHEN** a flex item declares a negative top margin
- **THEN** the import reports the dropped margin instead of silently removing it
