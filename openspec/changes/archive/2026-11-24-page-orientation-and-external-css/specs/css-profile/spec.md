# Spec Delta

## MODIFIED Requirements

### Requirement: Supported elements

The profile SHALL include the elements `html`, `head`, `body`, `div`, `p`, `h1`–`h6`, `ul`, `ol`, `li`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `td`, `th`, `img`, `span`, `br`, `hr`, `strong`, `b`, `em`, `i`, `a`, `link` (only as stylesheet inclusion) and `script` (only for ignoring). An unknown element SHALL be rejected with the element name and position.

#### Scenario: Allowed element
- **WHEN** the template contains an `h2` element
- **THEN** validation accepts the element, and layout applies the default styles from the engine's stylesheet to it

#### Scenario: Unknown element
- **WHEN** the template contains a `video` element
- **THEN** validation fails with the element name and its position

#### Scenario: Stylesheet inclusion
- **WHEN** the template contains `<link rel="stylesheet" href="styles/print.css">`
- **THEN** validation accepts the element, and the styles from the file participate in the cascade

#### Scenario: A link that is not a stylesheet
- **WHEN** the template contains `<link rel="icon" href="favicon.ico">`
- **THEN** validation emits no diagnostics, and the element does not affect layout

## ADDED Requirements

### Requirement: Local stylesheet via a link

An `link` element with `rel="stylesheet"` and a local `href` SHALL include the stylesheet in the cascade in order of appearance in the document. The `media` attribute with the value `print`, `all` or empty SHALL apply the sheet; any other value SHALL cause the sheet to be skipped without diagnostics.

#### Scenario: Cascade order
- **WHEN** a document declares a `<style>` with a color, then a `<link>` with another color, then another `<style>`
- **THEN** the color from the last declaration is applied, as in a normal CSS cascade

#### Scenario: Print-only sheet
- **WHEN** `<link rel="stylesheet" media="print" href="print.css">` sets a padding
- **THEN** the padding is applied when building the PDF

#### Scenario: Screen sheet
- **WHEN** `<link rel="stylesheet" media="screen" href="screen.css">` sets a padding
- **THEN** the sheet is skipped, the build succeeds, there are no diagnostics

#### Scenario: Ordinary link
- **WHEN** the document contains `<link rel="canonical" href="https://example.com/">`
- **THEN** the element is ignored, there are no diagnostics

### Requirement: Path confinement and external style diagnostics

Remote style sources and `data:` URLs SHALL be rejected. A missing local file SHALL produce an error indicating the path. If a base directory is set, the resolved path SHALL remain inside it. A diagnostic for a declaration from an external file SHALL indicate the file, the line and the column in that file.

#### Scenario: Remote style
- **WHEN** `<link rel="stylesheet" href="https://example.com/site.css">`
- **THEN** validation fails with `css/link-remote`, no PDF is created

#### Scenario: Missing file
- **WHEN** `<link rel="stylesheet" href="styles/missing.css">`
- **THEN** the error `css/link-load` is emitted with the file path

#### Scenario: Escaping the base directory
- **WHEN** with a base directory set the template references `../../outside.css`
- **THEN** validation fails, the file is not read

#### Scenario: Error inside an included file
- **WHEN** `display: flex` is declared on line 12 in `styles/print.css`
- **THEN** the diagnostic indicates `styles/print.css:12` with a column, and not an HTML line

### Requirement: Ignoring scripts

The `script` element SHALL be ignored: its content is neither executed nor involved in layout. Validation SHALL emit the warning `html/ignored-script` with the element position. In strict mode this warning SHALL become an error.

#### Scenario: Script in a template
- **WHEN** the template contains `<script>console.log("x")</script>` and text
- **THEN** the build succeeds, a warning is emitted, and the script content does not get into the PDF

#### Scenario: Strict mode
- **WHEN** the build is run with `-strict` and the template contains `<script>`
- **THEN** validation fails, no PDF is created

#### Scenario: Markup inside a template script
- **WHEN** the script is declared as `<script type="text/template"><p>template</p></script>`
- **THEN** the markup inside is not rendered, but it also does not cause an error
