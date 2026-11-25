# Spec Delta

## ADDED Requirements

### Requirement: CSS string escapes are decoded

The CSS tokenizer SHALL decode backslash escapes in strings and identifiers: one to
six hexadecimal digits followed by an optional space, and a backslash-newline
continuation. The decoded value MUST be used by `content` and `font-family`; a
sequence such as `"\2014"` MUST NOT reach the renderer as the characters `2014`.

#### Scenario: Hexadecimal escapes in content

- **WHEN** `a::after { content: "\2014" }` is converted
- **THEN** the generated text is an em dash

#### Scenario: Escaped space

- **WHEN** the value is `"A\20 B"`
- **THEN** the decoded text is `A B`

### Requirement: Border colours accept the values color accepts

The `border` shorthand SHALL accept every colour form that the `color` property
accepts, including `rgb()` and hexadecimal values. `transparent` SHALL paint the
border with no visible pixels instead of black.

#### Scenario: An rgb border

- **WHEN** a rule declares `border: 1px solid rgb(255, 0, 0)`
- **THEN** validation accepts the declaration and the border is red

#### Scenario: A transparent border

- **WHEN** a rule declares `border: 1px solid transparent`
- **THEN** no visible border is painted
