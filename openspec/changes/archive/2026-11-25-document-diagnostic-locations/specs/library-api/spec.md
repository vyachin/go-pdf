# Spec Delta

## MODIFIED Requirements

### Requirement: Diagnostics identify the converted document

Document and asset diagnostics SHALL name their source file, or `Options.File` /
the placeholder when no more specific file is known. Attributable failures MUST
retain known line and column, including missing image/font references. Option
errors MAY omit file and position. Aggregate or output failures without a source
location MAY use zero coordinates but MUST name the document. Unknown locations
MUST NOT be fabricated.

#### Scenario: A missing image

- **WHEN** a document named `invoice.html` references a missing image
- **THEN** the `img/load` diagnostic has `File == "invoice.html"` and a position

#### Scenario: A resource limit

- **WHEN** a document named `invoice.html` exhausts `MaxOperations`
- **THEN** the `resource/limit` diagnostic names `invoice.html`

#### Scenario: A rule in a linked stylesheet

- **WHEN** a linked stylesheet contains an unsupported declaration
- **THEN** the diagnostic names that stylesheet with its line and column

#### Scenario: An aggregate output failure has no source location

- **WHEN** PDF serialization exhausts a conversion-wide budget without identifying one source node
- **THEN** the `resource/limit` diagnostic names the document
- **AND** line and column may be zero to mean unknown without fabricating an element path
