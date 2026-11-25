# Spec Delta

## ADDED Requirements

### Requirement: Asset policy is consistent within a conversion

Permitted references to one image or font file SHALL share one successful asset
record and count bytes once per conversion. Identity MUST distinguish kind and
decoding inputs; every reuse MUST enforce the reference's root and access policy.
Stylesheet inclusions MUST retain cascade order. State MUST be conversion-local,
released on every exit, and preserve typed failures with their source and cause.

#### Scenario: One font has two permitted roles

- **WHEN** a permitted canonical font file is used by `@font-face` and explicit `FallbackFont` in one conversion
- **THEN** the bytes are read, parsed and admitted once while both roles resolve correctly

#### Scenario: A cached file is outside another root

- **WHEN** one reference is permitted to load a file and a later template reference would escape its own configured root
- **THEN** the later reference is rejected even if the file is already cached

#### Scenario: A stylesheet is included twice

- **WHEN** a local stylesheet is linked twice with another sheet between its inclusions
- **THEN** both inclusions participate at their original cascade positions even if file reading and immutable parsing are reused

#### Scenario: Independent conversions use different assets

- **WHEN** two concurrent conversions use equal relative names under different roots
- **THEN** their records, totals, failures and output remain isolated

#### Scenario: A resource load fails

- **WHEN** an asset cannot be loaded or exceeds a limit
- **THEN** the typed failure retains its resource kind, original reference and cause and unsuccessful reads are not committed as successful assets
