# Spec Delta

## ADDED Requirements

### Requirement: A node's source span comes from its own token (RV4-10)

A node's text span and source position MUST be the span and position of its own
token, including under foster parenting. A `<style>` element's content MUST be the
CSS text written inside that element. Tree order and source order MAY differ; the
recorded spans MUST NOT.

#### Scenario: A style element inside a table

- **WHEN** a document contains
  `<table><style>p{color:red}</style>p{color:blue}<tr><td>y</td></tr></table>`
- **THEN** the stylesheet contains the rule `p{color:red}`
- **AND** the foster-parented text `p{color:blue}` is not parsed as CSS
- **AND** no diagnostic is produced

#### Scenario: Positions stay with their element

- **WHEN** a fostered element and a cell element carry the same tag and attributes
- **THEN** each diagnostic points at the line and column of its own element

### Requirement: A declared windows-1252 is rejected even for valid UTF-8 bytes (RV4-17)

A document that declares windows-1252 SHALL fail with `html/encoding` regardless of
whether its bytes happen to be valid UTF-8. Only the no-declaration fallback may be
accepted as UTF-8, and only when the whole input is valid UTF-8.

#### Scenario: A windows-1252 declaration over UTF-8 bytes

- **WHEN** a document declares `<meta charset="windows-1252">` and its body is valid
  UTF-8 Cyrillic
- **THEN** validation fails with `html/encoding` naming windows-1252
- **AND** no PDF is produced

### Requirement: The meta position survives case folding (RV4-38)

A diagnostic that points at a `<meta>` declaration SHALL report the byte offset of
that declaration in the original input. Case-insensitive matching MUST NOT change
byte lengths or shift the reported column.

#### Scenario: A declaration after a multi-byte word

- **WHEN** the first 1024 bytes contain a multi-byte word and then
  `<meta charset="big5">`
- **THEN** the diagnostic points at the `<meta>` byte offset
- **AND** the same holds for a declaration after a rune whose lower-case form has a
  different byte length

### Requirement: Diagnostic columns have one defined meaning (RV4-39)

The package SHALL define whether a diagnostic column counts bytes or characters in
the original file, use that definition consistently for UTF-8 and legacy encodings,
and state it in the documentation. A test MUST pin the choice for a line with
multi-byte characters.

#### Scenario: A position after Cyrillic text

- **WHEN** `<p>Привет</p><img src="missing.png">` produces an `img/load` diagnostic
- **THEN** the column follows the documented rule
- **AND** the README states the same rule

### Requirement: Source spans are recorded while the tree is built (IMP-04)

The parser SHALL record every node's byte span and position from its token while the
tree is built, not reconstruct them by walking the tree. A node the parser implies
SHALL inherit the range of its nearest ancestor, and a sliced raw-text span MUST be
verified against the node's text before it is used.

#### Scenario: A new node kind (IMP-04)

- **WHEN** a new element or raw-text kind is added
- **THEN** its span comes from its own token
- **AND** the foster-parenting regression test stays green

