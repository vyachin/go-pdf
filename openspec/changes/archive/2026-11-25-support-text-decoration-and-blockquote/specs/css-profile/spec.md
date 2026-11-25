# Spec Delta

## MODIFIED Requirements

### Requirement: Supported elements

The profile SHALL include the elements `html`, `head`, `body`, `main`, `div`, `p`, `h1`–`h6`, `ul`, `ol`, `li`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `td`, `th`, `img`, `span`, `br`, `hr`, `strong`, `b`, `em`, `i`, `u`, `s`, `del`, `ins`, `blockquote`, `a`, `link` (only as a stylesheet reference) and `script` (only to be ignored). An unknown element SHALL be rejected with its name and position named.

#### Scenario: An accepted element
- **WHEN** the template contains an `h2` element
- **THEN** validation accepts the element and layout applies the engine stylesheet's default styles to it

#### Scenario: A document landmark
- **WHEN** the template contains a `main` element
- **THEN** validation accepts the element and layout gives it block layout like a `div`

#### Scenario: An unknown element
- **WHEN** the template contains a `video` element
- **THEN** validation fails with the element name and its position named

#### Scenario: Referencing a stylesheet
- **WHEN** the template contains `<link rel="stylesheet" href="styles/print.css">`
- **THEN** validation accepts the element and the styles from the file take part in the cascade

#### Scenario: A link that is not a stylesheet
- **WHEN** the template contains `<link rel="icon" href="favicon.ico">`
- **THEN** validation emits no diagnostics and the element does not affect layout

#### Scenario: A decoration element
- **WHEN** the template contains `u`, `s`, `del` or `ins`
- **THEN** validation accepts each element and the engine stylesheet underlines `u` and `ins` and strikes through `s` and `del`

### Requirement: Supported text properties

The profile SHALL include the properties `color`, `font-family`, `font-size`, `font-weight`, `font-style`, `line-height`, `text-align`, `text-indent`, `white-space`, `vertical-align` and `text-decoration`. Property values outside the profile SHALL be rejected.

#### Scenario: Typeface and size
- **WHEN** the template sets `font-family: "PT Serif"`, `font-size: 12pt` and `font-weight: 700`
- **THEN** validation accepts the declarations and the text is rendered in the specified typeface

#### Scenario: Unsupported white-space value
- **WHEN** the template sets `white-space: pre-wrap`
- **THEN** validation fails with the unsupported value named

#### Scenario: A decoration value
- **WHEN** the template sets `text-decoration: underline`, `none`, `line-through` or `underline line-through`
- **THEN** validation accepts the declaration and every other keyword is rejected with the value named

#### Scenario: A decoration is inherited
- **WHEN** a decorated element contains a child element with no decoration of its own
- **THEN** the child's text carries the same decoration
