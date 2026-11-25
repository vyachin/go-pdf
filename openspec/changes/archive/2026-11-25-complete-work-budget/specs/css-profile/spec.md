# Spec Delta

## ADDED Requirements

### Requirement: Selector matching is bounded and accounted

Matching a supported selector against an element SHALL take time bounded by the number
of compounds and the element's ancestor depth, including selectors that use the
adjacent-sibling combinator `+`. The work of matching MUST be recorded in the operation
budget, and a conversion whose budget is exhausted or whose context is cancelled while
a selector is matched MUST stop with the documented `resource/limit` diagnostic or
context error.

#### Scenario: Adjacent selector on a deep tree

- **WHEN** a stylesheet contains a selector with three or more compounds that includes
  `+`, and the document nests elements deeper than the selector's descendant count
- **THEN** conversion finishes instead of spending time exponential in the depth
- **AND** the document renders with the styles the selector matches

#### Scenario: Matching work is counted

- **WHEN** a document requires more selector-matching work than its operation budget
  allows
- **THEN** the call stops with a `resource/limit` diagnostic

#### Scenario: Cancellation during matching

- **WHEN** the context is cancelled while a selector is matched against an element
- **THEN** the call returns the context error
- **AND** the error is not turned into a document diagnostic

#### Scenario: Results do not change

- **WHEN** an adjacent selector is matched against a document
- **THEN** the matched elements are exactly those the unoptimised matcher would return
