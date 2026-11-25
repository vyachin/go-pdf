# Spec Delta

## ADDED Requirements

### Requirement: Attributes are validated and hidden hides content

The profile SHALL validate attributes as well as elements and properties: an
attribute outside the documented allow-list MUST be reported with its position, and
the HTML `hidden` attribute MUST remove its subtree from layout. Content marked
`hidden` MUST NOT appear in the PDF.

#### Scenario: A hidden block

- **WHEN** a document contains `<div hidden><p>SECRET</p></div>`
- **THEN** the extracted PDF text does not contain "SECRET"

#### Scenario: An unknown attribute

- **WHEN** an element carries `data-x` or `onclick`
- **THEN** the diagnostic names the attribute and its position

#### Scenario: An allowed attribute

- **WHEN** an element carries a documented attribute such as `colspan` or `href`
- **THEN** no attribute diagnostic is produced
