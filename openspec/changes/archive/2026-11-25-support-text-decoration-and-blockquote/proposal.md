# Proposal

## Why

`docs/profile.md` lists `text-decoration` (`u`, `s`, `del`, `ins`) and
`<blockquote>` under "Known gaps": the elements are rejected by the closed
profile, so ordinary business documents cannot underline, strike through or
quote text even though mPDF renders those constructs. The profile is a contract,
so closing the gap means a requirement, a default rule in the engine stylesheet,
a painted result and a snapshot — not an ad-hoc parser exception.

## What Changes

- Accept the inline elements `u`, `s`, `del`, `ins` and the block element
  `blockquote`, with default rules that underline the first two, strike through
  the second two and indent the quote.
- Accept the `text-decoration` property with `none`, `underline`,
  `line-through` and any comma-free combination of the two keywords; the
  decoration propagates to descendant text, as CSS specifies for a decorated
  element.
- Paint a decoration line per text run on the correct side of the baseline,
  using the run's colour, and keep the recording of the decoration in the
  document snapshot.
- Move the two entries out of "Known gaps" in `docs/profile.md` and document the
  accepted values, the default rules and the painting rule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: accept the elements and the `text-decoration` property with its
  values, propagation and default rules.

## Impact

`internal/css/props.go` owns `SupportedElements` and the property/`PropOrder`
tables; `internal/css/ua.go` holds the engine stylesheet; `internal/css/style.go`
carries the computed style; `internal/layout/paint.go` emits the drawing
operations. `internal/engine/docs_facts_test.go` and the profile-catalogue test
keep the document and the tables in step, and `TestProfileCoverage` requires a
snapshot fixture for a new element or property.

No public Go identifiers change and no dependency is added: `Options` and the
diagnostic codes stay as they are.
