# Spec Delta

## ADDED Requirements

### Requirement: Aggregate asset limits

`Limits` SHALL bound the totals of one conversion in addition to each file: total
encoded image bytes, total decoded image pixels and total font bytes. An asset that
would cross a total MUST fail with a `resource/limit` diagnostic before it is read or
decoded, and the defaults MUST let a normal business document convert without
configuration.

#### Scenario: Many assets cross the total

- **WHEN** a document references assets whose sum exceeds a configured total while
  each asset is under its per-file limit
- **THEN** the call returns a `resource/limit` diagnostic
- **AND** the assets past the limit are not read or decoded

#### Scenario: A normal document stays under the totals

- **WHEN** a document with one logo, one body font and one bold face converts with
  the default limits
- **THEN** the conversion succeeds and the assets are embedded once each

#### Scenario: The caller can raise a total

- **WHEN** the caller sets a larger total in `Limits`
- **THEN** the document converts and the PDF is byte-identical to the same document
  converted before the limit was reached
