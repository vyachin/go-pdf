# Spec Delta

## ADDED Requirements

### Requirement: Inheritance follows the element chain

The engine SHALL compute styles along the real ancestor chain: `html` → `body`, and `table` → row group (`thead`, `tbody`, `tfoot`) → `tr` → `td`/`th`. Inherited properties set on an ancestor MUST reach its descendants through that chain, and attributes on the `html` element MUST survive parsing so selectors and inline styles on it take effect.

#### Scenario: Font size from html
- **WHEN** the stylesheet declares `html { font-size: 30pt }` and the body contains text without its own size
- **THEN** the text is rendered at 30 pt

#### Scenario: Colour from a table row
- **WHEN** the stylesheet declares `tr { color: red }`
- **THEN** the text in its cells is red

#### Scenario: Inline style on html
- **WHEN** the document declares `<html style="color: red">`
- **THEN** body text inherits the red colour

#### Scenario: Attributes survive parsing
- **WHEN** the `html` element carries a class or id
- **THEN** the attribute is present on the parsed element and selectors can match it
