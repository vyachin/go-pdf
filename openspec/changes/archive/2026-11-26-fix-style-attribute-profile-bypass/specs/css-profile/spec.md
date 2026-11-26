# Spec Delta

## ADDED Requirements

### Requirement: A style attribute is a declaration list, not a stylesheet (RV5-06)

A `style` attribute SHALL be parsed as a declaration list that ends only at end of
input. A `{` or `}` in it MUST produce `css/syntax`, and an at-rule MUST produce
`css/unsupported-at-rule`; neither may end the list silently or be discarded.
Declarations before the offending token SHALL stay parsed, validated and available
to the cascade, and every declaration MUST be either kept or reported.

#### Scenario: A leading brace before an unsupported declaration

- **WHEN** a document contains `<p style="} display:flex">x</p>`
- **THEN** validation reports `css/syntax` for the `}`
- **AND** it also reports `css/unsupported-value` for `display:flex`
- **AND** the call produces no PDF

#### Scenario: A brace in the middle of the attribute

- **WHEN** the attribute is `style="color:red} display:flex"`
- **THEN** validation reports `css/syntax` for the `}`
- **AND** `color:red` remains in the declaration list available to the cascade
- **AND** `display:flex` is reported instead of being dropped

#### Scenario: A stray opening brace

- **WHEN** the attribute is `style="color:red{display:flex"`
- **THEN** validation reports `css/syntax` for the `{`
- **AND** `display:flex` is reported as an unsupported value

#### Scenario: A media rule inside a style attribute

- **WHEN** the attribute is `style="@media print { p { display: flex } }"`
- **THEN** validation reports `css/unsupported-at-rule`
- **AND** the inner `display:flex` is not silently accepted

#### Scenario: A page rule inside a style attribute

- **WHEN** the attribute is `style="@page { size: A4 }"`
- **THEN** validation reports `css/unsupported-at-rule`
- **AND** no page setup is taken from the attribute

#### Scenario: A well-formed style attribute is unchanged

- **WHEN** the attribute is `style="color:#c00;font-size:10pt"`
- **THEN** validation reports nothing
- **AND** both declarations are applied as before
