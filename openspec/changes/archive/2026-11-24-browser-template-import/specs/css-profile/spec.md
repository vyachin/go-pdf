# Spec Delta

## MODIFIED Requirements

### Requirement: Supported elements

The profile SHALL include the elements `html`, `head`, `body`, `main`, `div`, `p`, `h1`–`h6`, `ul`, `ol`, `li`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `td`, `th`, `img`, `span`, `br`, `hr`, `strong`, `b`, `em`, `i`, `a`, `link` (only as a stylesheet reference) and `script` (only to be ignored). An unknown element SHALL be rejected with its name and position named.

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

### Requirement: Supported box model properties

The profile SHALL include the properties `display`, `width`, `height`, `margin`, `margin-top`, `margin-right`, `margin-bottom`, `margin-left`, `padding`, `padding-top`, `padding-right`, `padding-bottom`, `padding-left`, `border`, `border-collapse`, `background-color`, `background-image` and `background-size`. Property values outside the profile SHALL be rejected.

#### Scenario: Border and padding
- **WHEN** the template sets `padding: 4mm` and `border: 1pt solid #000`
- **THEN** validation accepts both declarations and layout applies them to the box

#### Scenario: A physical longhand after the shorthand
- **WHEN** the template sets `margin: 1mm 2mm 3mm 4mm` and then `margin-top: 5mm`
- **THEN** validation accepts both declarations and the top margin is 5 mm while the other sides keep their shorthand values

#### Scenario: A negative padding longhand
- **WHEN** the template sets `padding-left: -1mm`
- **THEN** validation fails with the property and the value named

#### Scenario: Unsupported display value
- **WHEN** the template sets `display: grid`
- **THEN** validation fails with the unsupported value named

#### Scenario: A background image that covers the box
- **WHEN** the template sets `background-image: url(cover.jpg)` and `background-size: cover`
- **THEN** validation accepts both declarations and layout paints the image behind the box's content

#### Scenario: A background image in a data URI
- **WHEN** the template sets `background-image: url("data:image/png;base64,…")`
- **THEN** validation accepts the declaration and the decoded image is painted behind the box's content

#### Scenario: A gradient background
- **WHEN** the template sets `background-image: linear-gradient(#000, #fff)`
- **THEN** validation fails with the property and the value named, and rendering does not start

#### Scenario: An unsupported background size
- **WHEN** the template sets `background-size: contain`
- **THEN** validation fails with the property and the value named

### Requirement: Supported selectors

The engine SHALL support selectors by element name, class and identifier, the universal `*`, their combinations, descendant (space), direct child (`>`), adjacent sibling (`+`), as well as comma-separated lists. Pseudo-classes and attribute selectors SHALL be rejected as unsupported, with the single exception of `:root`, which matches the document element. The single exception for pseudo-elements is described in a separate requirement.

#### Scenario: A combined selector
- **WHEN** the template contains `.invoice > td.total`
- **THEN** the rule applies to cells with class `total` inside a row with class `invoice`

#### Scenario: Adjacent siblings
- **WHEN** the template contains `p + p`
- **THEN** the rule applies to every `p` that immediately follows another `p` among the element siblings

#### Scenario: The document element
- **WHEN** the template contains `:root { color: #333 }`
- **THEN** the rule applies to the `html` element and not to its descendants

#### Scenario: Unsupported pseudo-class
- **WHEN** the template contains `tr:nth-child(2n)`
- **THEN** validation fails with the unsupported selector named

#### Scenario: Unsupported pseudo-element
- **WHEN** the template contains `p::before { color: red }`
- **THEN** validation fails, stating that only `a::after` is supported

### Requirement: Background image sources are local files

`background-image` SHALL accept a `url()` reference to a local file, resolved against the same configured root as an `<img>` source, or a base64 `data:image/png` / `data:image/jpeg` URI. A remote URL, a `data:` URI of any other type, or an absolute path outside the root SHALL be rejected with a diagnostic naming the source.

#### Scenario: A background image inside the base directory
- **WHEN** the fixture is converted with `BaseDir` set and `background-image: url(cover.jpg)` names a file inside it
- **THEN** the image is loaded from that file and embedded once

#### Scenario: A background image outside the base directory
- **WHEN** `background-image: url(../outside.jpg)` resolves outside `BaseDir`
- **THEN** conversion fails with a diagnostic naming the path, and the file is not read

#### Scenario: A remote background image
- **WHEN** the template sets `background-image: url(https://example.com/cover.jpg)`
- **THEN** conversion fails with a diagnostic naming the source, instead of fetching anything

#### Scenario: A vector data URI
- **WHEN** the template sets `background-image: url("data:image/svg+xml;base64,…")`
- **THEN** conversion fails with a diagnostic naming the image type, because SVG is not decoded
