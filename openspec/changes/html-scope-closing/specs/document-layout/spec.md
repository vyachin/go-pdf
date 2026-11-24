# Spec Delta

## ADDED Requirements

### Requirement: Nested tables and lists keep their structure

Implicit end tags SHALL be scoped: a cell, row or row group closes only inside its own table; a list item only inside its own list; `dt`/`dd` only inside their `dl`; `option` only inside its `select`. Text that follows a nested container MUST stay in the element that contains it.

#### Scenario: Nested table
- **WHEN** a table cell contains another table with its own row
- **THEN** the inner row belongs to the inner table
- **AND** the inner table stays inside the outer cell

#### Scenario: Nested list
- **WHEN** a list item contains a nested list with an item
- **THEN** the inner item belongs to the inner list
- **AND** text after the nested list stays in the outer item

#### Scenario: Flat table
- **WHEN** a table has several cells and rows without nesting
- **THEN** the cells keep their positions and the layout is unchanged
