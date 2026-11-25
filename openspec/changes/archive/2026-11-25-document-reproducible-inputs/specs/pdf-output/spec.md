# Spec Delta

## MODIFIED Requirements

### Requirement: Reproducible output

With the same engine build, HTML bytes, options and resolved asset bytes, the
engine SHALL produce byte-identical PDFs. Resolved assets MUST include linked
stylesheets, images and selected fonts under the same resolution choices. The
document identifier and creation date MUST be deterministic rather than read from
the clock. Different asset contents, font choices or engine builds are changed
inputs.

#### Scenario: Two runs produce the same file
- **WHEN** the same template is rendered twice
- **THEN** the checksums of both files match

#### Scenario: Explicit creation date
- **WHEN** the run specifies a document creation date
- **THEN** that date is stored in the PDF metadata instead of the current system time

#### Scenario: Deterministic document identifier
- **WHEN** two conversions use the same complete inputs
- **THEN** both files carry the same `/ID`, because it is a hash of the content rather than a random or clock value

#### Scenario: External resources are unchanged

- **WHEN** two conversions use the same engine build, HTML, options and identical resolved stylesheet, image and font bytes
- **THEN** the PDFs are byte identical even when run at different wall-clock times

#### Scenario: A resource changes in place

- **WHEN** an image or linked stylesheet changes bytes at the same configured path
- **THEN** the request has different complete inputs and the unchanged-input equality guarantee does not apply

#### Scenario: A different system font is selected

- **WHEN** two environments select different fallback font bytes
- **THEN** the documentation does not promise identical PDF bytes for those different resolved inputs
