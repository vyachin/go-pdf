# Spec Delta

## ADDED Requirements

### Requirement: HTML image dimensions are presentation hints

Valid `width` and `height` attributes on `img` SHALL be non-negative integer CSS
pixel presentation hints. Author CSS MUST override the corresponding attribute,
including an explicit `auto`. A malformed, negative, percentage or out-of-range
attribute MUST be ignored as an absent hint without integer overflow. Zero MUST
produce a non-visible dimension. With one effective dimension the other SHALL
follow the intrinsic aspect ratio.

#### Scenario: HTML dimensions affect the output

- **WHEN** the same image is rendered with `width="10" height="10"` and with `width="200" height="200"`
- **THEN** the painted rectangles have the respective CSS-pixel dimensions
- **AND** the 10 by 10 rectangle matches `width:10px;height:10px`

#### Scenario: CSS overrides a dimension hint

- **WHEN** an image has `width="200"` and author CSS `width:10px`
- **THEN** its used width is 10 CSS pixels

#### Scenario: An explicit auto overrides the attribute

- **WHEN** an image has `width="200"` and author CSS `width:auto`
- **THEN** the HTML width hint does not determine its used width

#### Scenario: One dimension preserves the aspect ratio

- **WHEN** a 200 by 100 pixel image has only `width="50"`
- **THEN** its used dimensions are 50 by 25 CSS pixels

#### Scenario: Invalid dimensions do not wrap geometry

- **WHEN** image attributes are negative, malformed, percentages or larger than the geometry range
- **THEN** they behave as absent hints and no overflowing or negative dimension reaches painting
