# Spec Delta

## Purpose

Defines which subset of HTML and CSS the engine must understand and how it reports the unsupported. The profile is a contract: a template is either valid as a whole or receives a diagnostic before rendering starts.

## ADDED Requirements

### Requirement: Profile as a closed allow-list

The engine SHALL accept only the elements, properties, values, units and at-rules listed in the v1 profile. Any unsupported property, value, unit or at-rule MUST lead to a validation error rather than to silent ignoring.

#### Scenario: Unsupported display value
- **WHEN** the template contains `div { display: flex }`
- **THEN** validation fails, naming the property, the value, the file and the position
- **AND** rendering does not start

#### Scenario: Unsupported property
- **WHEN** the template contains `box-shadow: 0 0 4px #000`
- **THEN** validation fails, stating that the property is not part of the v1 profile

#### Scenario: Fully supported template
- **WHEN** all elements and properties of the template are part of the profile
- **THEN** validation succeeds without diagnostics

### Requirement: Supported elements

The profile SHALL include the elements `html`, `head`, `body`, `div`, `p`, `h1`–`h6`, `ul`, `ol`, `li`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `td`, `th`, `img`, `span`, `br`, `hr`, `strong`, `b`, `em`, `i`, `a`. An unknown element MUST be rejected with its name and position.

#### Scenario: Allowed element
- **WHEN** the template contains an `h2` element
- **THEN** validation accepts the element, and layout applies the default styles from the engine stylesheet to it

#### Scenario: Unknown element
- **WHEN** the template contains a `video` element
- **THEN** validation fails, naming the element and its position

### Requirement: Supported box model properties

The profile SHALL include the properties `display`, `width`, `height`, `margin`, `padding`, `border`, `border-collapse` and `background-color`. Property values outside the profile MUST be rejected.

#### Scenario: Border and padding
- **WHEN** the template sets `padding: 4mm` and `border: 1pt solid #000`
- **THEN** validation accepts both declarations and layout applies them to the box

#### Scenario: Unsupported display value
- **WHEN** the template sets `display: grid`
- **THEN** validation fails, naming the unsupported value

### Requirement: Supported text properties

The profile SHALL include the properties `color`, `font-family`, `font-size`, `font-weight`, `font-style`, `line-height`, `text-align`, `text-indent`, `white-space` and `vertical-align`. Property values outside the profile MUST be rejected.

#### Scenario: Style and size
- **WHEN** the template sets `font-family: "PT Serif"`, `font-size: 12pt` and `font-weight: 700`
- **THEN** validation accepts the declarations, and the text is rendered in the specified style

#### Scenario: Unsupported white-space value
- **WHEN** the template sets `white-space: pre-wrap`
- **THEN** validation fails, naming the unsupported value

### Requirement: Supported break and list properties

The profile SHALL include the properties `break-before`, `break-after`, `break-inside`, `page-break-before`, `page-break-after`, `page-break-inside`, `orphans`, `widows` and `list-style-type`.

#### Scenario: Prohibition of a break in a template
- **WHEN** the template sets `page-break-inside: avoid` for a table row
- **THEN** validation accepts the declaration and passes it to pagination

#### Scenario: Unsupported list-style-type value
- **WHEN** the template sets `list-style-type: lower-roman`
- **THEN** validation fails, naming the unsupported value

### Requirement: Supported units of measurement

The engine SHALL support the units `px`, `pt`, `mm`, `cm`, `in`, `%` and `em`. Percentages MUST resolve against the containing block, `em` against the computed `font-size` of the element. The units `rem`, `vw`, `vh`, `ch` MUST be rejected as unsupported.

#### Scenario: Mixed units
- **WHEN** the template sets `width: 100mm` and `padding: 1.5em` at `font-size: 12pt`
- **THEN** widths are converted into a single internal unit without loss of precision
- **AND** the block width equals 100 mm

#### Scenario: Unsupported unit
- **WHEN** the template contains `margin: 2rem`
- **THEN** validation fails, naming the unit `rem`

### Requirement: Cascade order and inheritance

The engine SHALL resolve style conflicts in the order: browser default styles, then author styles by increasing selector specificity, then the `style` attribute. With equal specificity, the declaration made later MUST win. Shorthand properties MUST expand into longhand ones.

#### Scenario: Inline style beats a class selector
- **WHEN** `td` has a class with `color: red` and the attribute `style="color: blue"`
- **THEN** the resulting text colour is blue

#### Scenario: A later rule with equal specificity
- **WHEN** two rules with the same specificity set a different `font-size`
- **THEN** the value from the rule declared last applies

### Requirement: Supported selectors

The engine SHALL support selectors by element name, class, id, their combination, descendant (space) and direct child (`>`), as well as a comma-separated list. Pseudo-classes and attribute selectors MUST be rejected as unsupported. The single exception for pseudo-elements is described in a separate requirement.

#### Scenario: Combined selector
- **WHEN** the template contains `.invoice > td.total`
- **THEN** the rule applies to cells with class `total` inside a row with class `invoice`

#### Scenario: Unsupported pseudo-class
- **WHEN** the template contains `tr:nth-child(2n)`
- **THEN** validation fails, naming the unsupported selector

#### Scenario: Unsupported pseudo-element
- **WHEN** the template contains `p::before { color: red }`
- **THEN** validation fails, stating that only `a::after` is supported

### Requirement: Diagnostics are tied to the source

Every diagnostic MUST contain the file, the line, the column and the element path in the document tree. The engine SHALL report all errors found in one validation pass, not only the first. The order of diagnostics MUST be deterministic.

#### Scenario: Several errors in one pass
- **WHEN** the template contains three unsupported declarations in different places
- **THEN** the report contains three diagnostics with independent coordinates

#### Scenario: A repeated run gives the same report
- **WHEN** validation of the same template runs twice
- **THEN** the list of diagnostics matches in composition and order

### Requirement: The `a::after` pseudo-element for generated content

The engine SHALL support exactly one pseudo-element — `a::after` — and only for the `content` property. The `content` value MUST consist of strings, `counter(page)`, `counter(pages)` and `target-counter(attr(href), page)`. Any other selector with a pseudo-element or any other property inside `a::after` MUST be rejected. Outside `a::after` the `content` property is not supported.

#### Scenario: Page number in the table of contents
- **WHEN** the template contains `a.toc::after { content: target-counter(attr(href), page) }`
- **THEN** validation accepts the rule, and after rendering the target page number is appended to the link text

#### Scenario: Another pseudo-element
- **WHEN** the template contains `p::before { content: "x" }`
- **THEN** validation fails with `css/unsupported-selector`

#### Scenario: A property inside a::after
- **WHEN** the template contains `a::after { color: red }`
- **THEN** validation fails with `css/unsupported-property`

#### Scenario: content outside a::after
- **WHEN** the template contains `p { content: "x" }`
- **THEN** validation fails with `css/unsupported-property`

### Requirement: Profile validation without rendering

The engine SHALL provide a template check mode that finishes without creating a PDF, with a non-zero exit code when there are errors. This mode MUST be suitable for running in CI.

#### Scenario: Checking a valid template
- **WHEN** a check runs on a template that is fully inside the profile
- **THEN** the exit code is zero and no PDF is created

#### Scenario: Checking a template with an error
- **WHEN** a check runs on a template with an unsupported property
- **THEN** the exit code is non-zero, the diagnostic is printed and no PDF is created
