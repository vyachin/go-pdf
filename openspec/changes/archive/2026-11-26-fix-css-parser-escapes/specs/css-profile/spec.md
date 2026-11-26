# Spec Delta

## ADDED Requirements

### Requirement: Selector escapes decode to data, not to syntax (RV5-26)

A character produced by an escape in a selector SHALL be part of the name it was
written in and MUST NOT be re-read as selector syntax. `.a\.b` MUST be one class
selector matching `class="a.b"`, and `.a\ b` MUST be one class name containing a
space rather than a descendant selector.

#### Scenario: A class name containing a dot

- **WHEN** the stylesheet contains `.a\.b { color:#c00 }` and the document contains
  `<p class="a.b">x</p>`
- **THEN** the rule matches the paragraph and the text is red

#### Scenario: The escaped dot is not a class separator

- **WHEN** the document contains `<p class="a b">x</p>` and no element with the
  single class `a.b`
- **THEN** `.a\.b` does not match that paragraph

#### Scenario: An escaped space

- **WHEN** the stylesheet contains `.a\ b { color:#c00 }`
- **THEN** the selector is one compound whose class name is `a b`
- **AND** it is not parsed as a descendant selector

#### Scenario: A plain compound is unchanged

- **WHEN** the stylesheet contains `.a.b { color:#c00 }`
- **THEN** it still requires both classes `a` and `b` on one element

### Requirement: url() decodes escapes and honours an escaped parenthesis (RV5-27)

A `url()` argument SHALL have its CSS escapes decoded whether it is quoted or
unquoted, and an unquoted `\)` MUST close the argument. `url("a\20 b.png")` MUST name
the same file as `url("a b.png")`, and a decoded escape MUST NOT reach the asset
loader as part of a literal path.

#### Scenario: An escaped space in a quoted url

- **WHEN** the stylesheet contains `background-image: url("a\20 b.png")` and the
  directory holds a file named `a b.png`
- **THEN** the image loads and is painted
- **AND** the conversion does not report `img/load`

#### Scenario: An escaped closing parenthesis, unquoted

- **WHEN** the value is `background-image: url(a\)b.png)` and the directory holds a
  file named `a)b.png`
- **THEN** the argument is `a)b.png` and the image loads

#### Scenario: Other sources are still rejected

- **WHEN** the value is a remote URL or a `data:` URI the profile does not accept
- **THEN** validation still reports the existing diagnostic
- **AND** the escape handling does not widen the accepted sources

#### Scenario: A plain url is unchanged

- **WHEN** the value is `url(cover.jpg)`
- **THEN** it resolves exactly as before
