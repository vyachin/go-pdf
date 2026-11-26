# Spec Delta

## ADDED Requirements

### Requirement: The work cost model accounts for bytes (RV5-11)

Every stage whose memory grows with the input SHALL charge the work guard before it
allocates, in proportion to the bytes it is about to materialise. One unit SHALL
correspond to a documented maximum number of bytes, and
`TestAdversarialSpendIsProportional` MUST cover line indexing, decoded text and layout
glyph copies. A document inside `MaxInputBytes` MUST fail with `resource/limit` before
an allocation exceeds that documented multiple.

#### Scenario: Eight MiB of text under a small budget

- **WHEN** a document contains 8 MiB of text and `MaxOperations` is 100000
- **THEN** the call returns `resource/limit`
- **AND** the bytes allocated before the stop are within the documented multiple of
  the budget, not of the input

#### Scenario: Spending is proportional for every covered stage

- **WHEN** `TestAdversarialSpendIsProportional` runs its fixtures at a fixed
  `MaxOperations`
- **THEN** each stage's allocated bytes stay within the documented bytes-per-unit
  bound
- **AND** the test names the stage that crossed it

#### Scenario: In-budget documents are unchanged

- **WHEN** a business document converts inside the default limits
- **THEN** its PDF and diagnostics are byte-identical to the result before this change
