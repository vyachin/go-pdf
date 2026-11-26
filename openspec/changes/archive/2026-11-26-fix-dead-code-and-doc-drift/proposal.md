# Proposal

## Why

Three drift problems make the code and its documents claim more than they do.
`internal/html` carries `InsertBefore`, `InsertAt`, `relinkFrom` and
`ChildIndex`, none of which has a caller, and the comment above `relinkFrom`
still says foster parenting lands there; `css.PropOrder` duplicates the keys of
`css.Table` with no test tying them together, and `Token.Text` claims to hold
the raw spelling although ident, hash and at-keyword tokens are decoded; the
profile document calls its attribute table complete but has no `<style>` row,
describes `media` as matching "contains print or all" when the implementation
matches whole list items, and omits percentage `line-height`. Each gap is a
future defect: a reader trusts a comment or a table that the code does not
enforce.

## What Changes

- Remove the unreferenced child-list helpers and the comment that describes
  foster parenting through them, or keep a helper only with a test that
  exercises its relinking (RV5-45).
- Make `PropOrder` provably equal to the key set of `Table`, and correct the
  `Token.Text` comment to name which token kinds are decoded; add an invariant
  test (RV5-46).
- Complete `docs/profile.md`: add the `<style>` attribute row, state whole-item
  `media` matching, document percentage `line-height`, and add a test that
  compares the element, attribute and inheritance tables with the code
  (RV5-47).

No public API or rendering change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the source invariants extend to unreferenced internal
  helpers, the property-order table and the tokenizer's documented contract.
- `library-docs`: the profile document becomes a checked table of the closed
  profile rather than a prose description.

## Impact

- `internal/html/node.go` (removal and comment), `internal/css/props.go` and
  `internal/css/token.go` (comment), `internal/css/*_test.go` (invariants).
- `internal/profile/docs_test.go` (table-to-code comparison) and
  `docs/profile.md`.
- No exported identifier, option, diagnostic or output change.
