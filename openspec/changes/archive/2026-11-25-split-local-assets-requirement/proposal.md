# Proposal

## Why

`openspec validate --all --strict` reports one warning: the requirement "Local
assets stay inside their configured root" in `css-profile` carries two rules in
one 549-character statement, past the length the validator accepts. Every strict
validation run of the project therefore ends with a known failure, which makes a
new warning indistinguishable from the old one.

## What Changes

- Split the requirement into two: asset confinement (which root each reference
  is opened inside) and read-failure attribution (a file that exists but cannot
  be read is reported with its reason, never as a boundary escape).
- Keep every existing scenario, word for word, under the requirement it belongs
  to; no behaviour, diagnostic code or documentation changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: replace one over-long requirement with two requirements that
  state one rule each.

## Impact

`openspec/specs/css-profile/spec.md` only. The engine, the diagnostics, the
profile document and the tests keep their current contract: this is a
specification-hygiene change that makes strict validation meaningful again.

No public Go identifier, dependency or runtime behaviour changes.
