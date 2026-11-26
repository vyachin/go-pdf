# Spec Delta

## MODIFIED Requirements

### Requirement: A node's source span comes from its own token (RV4-10)

A node's text span and source position MUST be the span and position of its own
token, including under foster parenting. Text SHALL be compared after character
references and line endings are decoded, so a token written with a reference and a
token written literally with the same text match. A `<style>` element's content
MUST be the CSS text written inside that element. Tree order and source order MAY
differ; the recorded spans MUST NOT.

#### Scenario: A style element inside a table

- **WHEN** a document contains
  `<table><style>p{color:red}</style>p{color:blue}<tr><td>y</td></tr></table>`
- **THEN** the stylesheet contains the rule `p{color:red}`
- **AND** the foster-parented text `p{color:blue}` is not parsed as CSS
- **AND** no diagnostic is produced

#### Scenario: Positions stay with their element

- **WHEN** a fostered element and a cell element carry the same tag and attributes
- **THEN** each diagnostic points at the line and column of its own element

#### Scenario: A text node with a character reference

- **WHEN** the document contains `<p>a &amp; b</p>`
- **THEN** the text node's data is `a & b`
- **AND** its range covers the source bytes `a &amp; b`
- **AND** a diagnostic raised for that text names its line and column, not the
  start tag of the `<p>` element

#### Scenario: A reference next to a literal ampersand

- **WHEN** the document contains `<p>&amp;</p><p>&</p>`
- **THEN** each text node carries its own token's range
- **AND** neither node reports the other paragraph's position

## ADDED Requirements

### Requirement: Text position matching is indexed

The parser SHALL match a text node to its token without scanning the document's
other tokens: a token next in source order uses the fast path, and an
out-of-source-order token uses an index of decoded token text. Matching work MUST
stay linear in the document size.

#### Scenario: A document of entity fragments

- **WHEN** a document repeats `<p>&amp;</p>` 16,000 times
- **THEN** the document parses
- **AND** the recorded work is within a constant multiple of its token count

#### Scenario: A foster-parented text among many tokens

- **WHEN** a document repeats a table whose `<style>` content follows a
  foster-parented text 1,000 times
- **THEN** each text node carries its own token's range
- **AND** the recorded work is within a constant multiple of the token count

### Requirement: Text position matching is charged and cancellable

Building the index, every out-of-source-order lookup, and every span a fallback
search examines SHALL be charged to the shared work budget. Cancellation observed
during that work MUST be returned as the context error, and a document whose
position matching exceeds the budget MUST fail with `resource/limit`.

#### Scenario: A budget exhausted while matching

- **WHEN** a document with many foster-parented text nodes is parsed with a work
  budget below the cost of matching them
- **THEN** the call fails with the work-limit error
- **AND** no document is returned

#### Scenario: Cancellation is returned

- **WHEN** the context is already cancelled when a document with foster-parented
  text is parsed
- **THEN** the call returns the context error, not a document diagnostic

### Requirement: A trimmed leading newline does not move a text node's range

When the tree construction drops a leading newline from the first text token of
`pre`, `listing` or `textarea`, the text node's range SHALL start at the first raw
byte after that newline. A newline written as a character reference SHALL be
accounted for by that reference's bytes.

#### Scenario: A preformatted block

- **WHEN** the document contains `<pre>\nX</pre>`
- **THEN** the text node's data is `X`
- **AND** its range covers `X`, not the dropped line feed

#### Scenario: A textarea with CRLF

- **WHEN** the document contains `<textarea>\r\nX</textarea>`
- **THEN** the text node's range starts after both bytes of the dropped CRLF

#### Scenario: A reference that decodes to a newline

- **WHEN** the document contains `<pre>&#10;X</pre>`
- **THEN** the text node's range starts after the reference's bytes
- **AND** covers `X`

### Requirement: A NUL does not move a text node's range

When the tree construction replaces a NUL with U+FFFD or removes it, the text node
SHALL keep the range of its own token. The NUL itself MAY lie inside that range.

#### Scenario: A NUL in body text

- **WHEN** the document contains `<p>a\x00b</p>`
- **THEN** the text node's data is `ab`
- **AND** its range covers the token `a\x00b`

#### Scenario: A NUL in foreign content

- **WHEN** the document contains `<svg>a\x00b</svg>`
- **THEN** the text node's data is `a` U+FFFD `b`
- **AND** its range covers the token `a\x00b`

#### Scenario: A NUL in table text

- **WHEN** the document contains `<table>a\x00b<tr><td>x</td></tr></table>`
- **THEN** the fostered text node's data is `ab`
- **AND** its range covers the token `a\x00b`

### Requirement: Equal out-of-source-order texts keep their own ranges

An out-of-source-order lookup SHALL return the first occurrence of the node's text
that no other node already holds, in document order. Two text nodes with equal
text MUST NOT be given the same span.

#### Scenario: A paragraph and a foster-parented text

- **WHEN** the document contains
  `<p>A</p><table><style>S</style>A<tr><td>Y</td></tr></table>`
- **THEN** the paragraph's text and the foster-parented `A` carry different ranges
- **AND** each range covers its own token

#### Scenario: Two foster-parented texts

- **WHEN** the document contains
  `<table><style>S</style>A<tr><td>Y</td></tr><div></div>A</table>`
- **THEN** the two foster-parented `A` nodes carry different ranges
- **AND** the ranges are in document order
