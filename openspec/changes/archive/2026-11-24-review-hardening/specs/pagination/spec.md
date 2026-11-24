# Spec Delta

## ADDED Requirements

### Requirement: Pagination stops when no progress is possible

When a page cannot accept any content, the engine SHALL stop paginating and report an error diagnostic. It MUST NOT return a successful document that lost content or consists of empty pages. A safety iteration limit MAY remain, but reaching it MUST be an error rather than a warning.

#### Scenario: Line taller than the page
- **WHEN** a template sets `line-height` larger than the content area of a page
- **THEN** conversion fails with a `layout/no-progress` diagnostic
- **AND** no PDF is written

#### Scenario: Empty output is never successful
- **WHEN** layout cannot place any content on a fresh page
- **THEN** the result contains an error diagnostic
- **AND** the conversion returns an error

#### Scenario: Content that fits keeps working
- **WHEN** a normal multi-page document is converted
- **THEN** conversion succeeds and the page count matches the content
