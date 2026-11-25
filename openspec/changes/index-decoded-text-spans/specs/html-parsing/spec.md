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

### Requirement: Text position matching uses a decoded index

The parser SHALL index the decoded text of its tokens and use that index when a
node's token is out of source order. A node whose token is next in source order
MUST be matched without scanning the document's other tokens, so matching work
stays linear in the document size.

#### Scenario: A document of entity fragments

- **WHEN** a document repeats `<p>&amp;</p>` 16,000 times
- **THEN** the document parses
- **AND** the recorded work is within a constant multiple of its token count

#### Scenario: A foster-parented text among many tokens

- **WHEN** a document repeats a table whose `<style>` content is moved after the
  table's foster-parented text 1,000 times
- **THEN** each text node carries its own token's range
- **AND** the recorded work is within a constant multiple of the token count

### Requirement: Text position matching is charged and cancellable

Building the decoded index, every out-of-order lookup, and every span a fallback
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
