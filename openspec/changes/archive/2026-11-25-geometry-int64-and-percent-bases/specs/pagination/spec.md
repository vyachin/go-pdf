# Spec Delta

## ADDED Requirements

### Requirement: Page margins are lengths and orientation flips a fallback

Margins declared in `@page` SHALL be non-negative lengths: `auto`, percentages and
negative values MUST be rejected with `css/unsupported-value`. An `em` margin SHALL
resolve against the initial font size, not the page width. `size: portrait` MUST swap
a landscape fallback size, and `size: landscape` MUST swap a portrait one.

#### Scenario: An em page margin

- **WHEN** `@page { size: A4; margin: 1em }` is converted
- **THEN** every margin is 12 pt, not 595 pt

#### Scenario: An auto page margin

- **WHEN** a template declares `@page { margin: auto }`
- **THEN** validation fails with `css/unsupported-value`

#### Scenario: A negative page margin

- **WHEN** a template declares `@page { margin: -20mm }`
- **THEN** validation fails with `css/unsupported-value`

#### Scenario: Portrait with a landscape fallback

- **WHEN** `Options.PageSize` is `300mmx200mm` and the document declares
  `@page { size: portrait }`
- **THEN** the page is 200 mm wide and 300 mm high
