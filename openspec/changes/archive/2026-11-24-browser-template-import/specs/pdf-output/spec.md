# Spec Delta

## ADDED Requirements

### Requirement: Data URI image sources

An `<img src>` and a `background-image` value SHALL accept a base64-encoded `data:image/png` or `data:image/jpeg` URI. The decoded bytes MUST pass through the same pixel and byte budgets as an image file, and a repeated URI SHALL be decoded once and embedded once, exactly like a repeated file. Any other data URI type, malformed base64 or a non-image data URI SHALL be rejected with a diagnostic naming the problem.

#### Scenario: An image embedded in the document
- **WHEN** a document contains `<img src="data:image/png;base64,…">`
- **THEN** the image is embedded in the PDF and the text layer is unaffected

#### Scenario: A background image in a data URI
- **WHEN** a box declares `background-image: url("data:image/jpeg;base64,…")`
- **THEN** the decoded JPEG is painted behind the box's content

#### Scenario: The same data URI twice
- **WHEN** the same data URI is used by an `<img>` and a background
- **THEN** the PDF contains one image object for it

#### Scenario: A vector data URI
- **WHEN** the source is `data:image/svg+xml;base64,…`
- **THEN** conversion fails with a diagnostic saying that only PNG and JPEG are decoded

#### Scenario: Malformed base64
- **WHEN** the payload is not valid base64
- **THEN** conversion fails with a diagnostic naming the malformed source
