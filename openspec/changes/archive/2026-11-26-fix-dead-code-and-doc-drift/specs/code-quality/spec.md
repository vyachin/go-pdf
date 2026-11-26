# Spec Delta

## ADDED Requirements

### Requirement: No unreferenced child-list mutation helpers (RV5-45)

Every helper in `internal/html` that mutates a node's child list SHALL be called
by production code or by a test, and the comment describing it MUST name an
existing caller. `InsertBefore`, `InsertAt`, `relinkFrom` and `ChildIndex` SHALL
be removed while they have no caller; a helper that survives MUST have a test
that exercises its relinking.

#### Scenario: Dead helpers are gone

- **WHEN** the package is built after the change
- **THEN** `InsertBefore`, `InsertAt`, `relinkFrom` and `ChildIndex` no longer exist
- **AND** the foster-parenting comment names no removed helper
- **AND** `go test ./internal/html` passes

#### Scenario: A kept helper is exercised

- **WHEN** a child-list mutation helper is kept for a real caller
- **THEN** a test calls it and asserts the resulting parent, sibling and
  previous-element links

### Requirement: Property order and token text are checked (RV5-46)

`PropOrder` SHALL contain exactly the keys of `Table`, and the `Token.Text`
documentation MUST state that ident, hash and at-keyword tokens hold decoded
text while strings and dimensions hold the source spelling. A test SHALL fail
when a property is added to one of the two collections only.

#### Scenario: A property is added to one collection

- **WHEN** a new property is added to `Table` without `PropOrder` or the other
  way round
- **THEN** the invariant test fails and names the missing name

#### Scenario: The token contract is documented

- **WHEN** the `Token.Text` comment is read
- **THEN** it names ident, hash and at-keyword as decoded
- **AND** it does not claim every kind holds the raw source spelling
