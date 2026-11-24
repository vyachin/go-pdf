# Spec Delta

## MODIFIED Requirements

### Requirement: Supported box model properties

The profile SHALL include the properties `display`, `width`, `height`, `margin`, `padding`, `border`, `border-collapse`, `background-color`, `background-image` and `background-size`. Property values outside the profile SHALL be rejected.

#### Scenario: Border and padding
- **WHEN** the template sets `padding: 4mm` and `border: 1pt solid #000`
- **THEN** validation accepts both declarations and layout applies them to the box

#### Scenario: Unsupported display value
- **WHEN** the template sets `display: grid`
- **THEN** validation fails with the unsupported value named

#### Scenario: A background image that covers the box
- **WHEN** the template sets `background-image: url(cover.jpg)` and `background-size: cover`
- **THEN** validation accepts both declarations and layout paints the image behind the box's content

#### Scenario: A gradient background
- **WHEN** the template sets `background-image: linear-gradient(#000, #fff)`
- **THEN** validation fails with the property and the value named, and rendering does not start

#### Scenario: An unsupported background size
- **WHEN** the template sets `background-size: contain`
- **THEN** validation fails with the property and the value named

## ADDED Requirements

### Requirement: Background image sources are local files

`background-image` SHALL accept only a `url()` reference to a local file, resolved against the same configured root as an `<img>` source. A remote URL, a `data:` URL or an absolute path outside the root SHALL be rejected with a diagnostic naming the source.

#### Scenario: A background image inside the base directory
- **WHEN** the fixture is converted with `BaseDir` set and `background-image: url(cover.jpg)` names a file inside it
- **THEN** the image is loaded from that file and embedded once

#### Scenario: A background image outside the base directory
- **WHEN** `background-image: url(../outside.jpg)` resolves outside `BaseDir`
- **THEN** conversion fails with a diagnostic naming the path, and the file is not read

#### Scenario: A remote background image
- **WHEN** the template sets `background-image: url(https://example.com/cover.jpg)`
- **THEN** conversion fails with a diagnostic naming the source, instead of fetching anything
