# Spec Delta

## ADDED Requirements

### Requirement: At-rule nesting is depth-limited

The CSS parser SHALL reject a stylesheet whose at-rules are nested deeper than 64
levels with a `resource/limit` diagnostic during parsing, before any recursive walk
over the rules. The limit SHALL apply to `@media`, page margin boxes and every other
nested at-rule.

#### Scenario: A deeply nested media query is rejected

- **WHEN** a document contains an inline stylesheet with 500,000 nested `@media`
  blocks, below the default input limit
- **THEN** the call returns a `resource/limit` diagnostic
- **AND** the process does not crash

#### Scenario: The limit does not reject valid nesting

- **WHEN** a stylesheet nests at-rules 64 levels deep
- **THEN** validation accepts the stylesheet and the declared rules apply

#### Scenario: One level over the limit

- **WHEN** a stylesheet nests at-rules 65 levels deep
- **THEN** the call returns a `resource/limit` diagnostic naming the nesting limit
