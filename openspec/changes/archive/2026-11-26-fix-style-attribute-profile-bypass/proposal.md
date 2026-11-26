# Proposal

## Why

`ParseDeclarations` appends a synthetic `}` token as an end sentinel and parses the
attribute with the stylesheet block parser. A real `}` in the attribute therefore
ends the declaration list silently, and nested rules that the block parser accepts
are returned to the caller and discarded. A document that writes
`<p style="} display:flex">` or `<p style="@media print { p { display: flex } }">`
validates with no diagnostics and renders, so the closed profile is bypassed: an
unsupported declaration never reaches validation, and the author is told nothing.
This contradicts the profile's allow-list and the requirement that no declaration
is dropped without a diagnostic.

## What Changes

- **BREAKING** for a template that relied on the bypass: a `{`, a `}` or an at-rule
  inside a `style` attribute is now reported instead of silently ending or being
  discarded. Such a template validates clean today and fails after the change.
- Parse a `style` attribute as a declaration list that ends only at end of input,
  not at a synthetic brace.
- Report `css/syntax` for `{` or `}` and `css/unsupported-at-rule` for an at-rule,
  keep the declarations that precede the offending token, and continue validation
  so that an unsupported declaration later in the attribute is still named.
- Add regression tests at the declaration parser, the profile and the public API.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the closed allow-list gains a specific rule that a style attribute
  is a declaration list, and that a brace or an at-rule in it is a reported problem
  rather than a silent terminator or a discarded rule.

## Impact

- `internal/css/parse.go` (`ParseDeclarations`, block parsing mode).
- `internal/profile/profile.go` and `internal/profile/profile_test.go`.
- `internal/css/css_test.go`, `internal/css/harden_test.go`.
- `internal/engine/convert_test.go` for the rendered outcome.
- No public API, output format or diagnostic code addition; existing codes carry
  the new problems.
