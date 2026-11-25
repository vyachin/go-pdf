# Spec Delta

## MODIFIED Requirements

### Requirement: Lists

The engine SHALL render markers for `list-style-type: disc` and numbering for `decimal`, aligning the marker outside the list item's text block. Nested lists SHALL receive an additional indent, and the `start` attribute SHALL set the initial number.

List numbering SHALL visit each sibling at most a constant number of times per
box-tree build. Every such traversal MUST be charged to the shared work guard and
MUST observe cancellation; numbering MUST NOT rescan each preceding sibling prefix.

#### Scenario: A numbered list with start
- **WHEN** an `ol` is declared with `start="5"` and contains two items
- **THEN** the items are numbered 5 and 6

#### Scenario: A nested list
- **WHEN** a list is nested inside an item of another list
- **THEN** its markers and text are indented to the right relative to the parent level

#### Scenario: A long shallow list has linear indexing work

- **WHEN** layout builds a list of 40,000 empty items
- **THEN** a deterministic traversal counter grows linearly with the number of siblings
- **AND** the existing marker and `start` semantics are preserved

#### Scenario: List traversal exhausts its budget

- **WHEN** the shared work budget cannot cover the next list traversal unit
- **THEN** conversion stops with `resource/limit` before that unit is processed

#### Scenario: Cancellation during list indexing

- **WHEN** the context is cancelled during a long list build
- **THEN** the call returns the context error without scanning every remaining prefix
