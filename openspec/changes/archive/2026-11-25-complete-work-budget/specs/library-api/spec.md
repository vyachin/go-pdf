# Spec Delta

## ADDED Requirements

### Requirement: The work budget covers every conversion stage

`Convert` and `Validate` SHALL charge the shared work guard for every stage whose
cost grows with the input: stylesheet handling, cascade and selector matching, font
parsing, image decoding, text shaping, layout and PDF serialisation. Once the budget
is exhausted the conversion MUST stop with a `resource/limit` diagnostic before the
stage continues, and a cancelled context MUST be observed during those stages rather
than after them.

#### Scenario: Image decoding is charged

- **WHEN** a document references images whose total pixels exceed the work budget
  while each image is under `MaxImagePixels`
- **THEN** the call returns a `resource/limit` diagnostic before decoding the images
  that would exceed the budget

#### Scenario: Font parsing is charged

- **WHEN** a font whose GPOS table is large enough to exhaust `MaxOperations` is
  loaded for a document
- **THEN** the call returns a `resource/limit` diagnostic instead of spending the
  full parse time

#### Scenario: Cancellation reaches a running stage

- **WHEN** the context is cancelled while a large image, font or page is processed
- **THEN** the call returns the context error, not a document diagnostic

#### Scenario: Within-budget work is unchanged

- **WHEN** a document converts inside the default limits
- **THEN** its PDF and diagnostics are byte-identical to the result before the
  change
