# Spec Delta

## MODIFIED Requirements

### Requirement: Arbitrary input never panics

`Convert` and `Validate` SHALL return either a result or an error for any byte sequence.
They MUST NOT panic, including on truncated HTML, malformed CSS, invalid images,
malformed fonts and deeply nested markup.

#### Scenario: Fuzzed input stays contained

- **WHEN** the fuzz target feeds arbitrary bytes to the public entry points
- **THEN** every iteration ends in a result or an error, and the process does not crash

#### Scenario: An over-limit document is rejected

- **WHEN** a document exceeds a configured limit, such as nesting depth or page count
- **THEN** the call returns a `resource/limit` diagnostic before the conversion model
  or layout allocates
