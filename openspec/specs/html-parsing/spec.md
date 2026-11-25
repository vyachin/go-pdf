# html-parsing Specification

## Purpose

Defines how an HTML document becomes the document model the profile validates and
the layout engine lays out: how character references are decoded, how the tree is
constructed, which normalizations the profile relies on, and what source position
each node carries.

## Requirements

### Requirement: Character references follow the HTML standard

Every named character reference defined by the HTML standard SHALL be decoded,
with and without a trailing semicolon where the standard allows it. A numeric
reference in the C1 range MUST be replaced by its windows-1252 character, and a
surrogate or out-of-range numeric reference by U+FFFD. A reference the standard
does not define SHALL be left as written, so it stays visible in the output.

#### Scenario: A named reference outside the common subset

- **WHEN** the document contains `&rarr;`
- **THEN** the text layer contains a rightwards arrow

#### Scenario: A legacy reference without a semicolon

- **WHEN** the document contains `&nbsp` followed by a space
- **THEN** the text layer contains a no-break space

#### Scenario: A windows-1252 numeric reference

- **WHEN** the document contains `&#147;`
- **THEN** the text layer contains a left double quotation mark

#### Scenario: An undefined reference

- **WHEN** the document contains `&nosuchentity;`
- **THEN** the text layer contains that sequence unchanged

### Requirement: The document tree is built by the standard algorithm

The tree SHALL be produced by the HTML standard's tree-construction algorithm
rather than by heuristics of our own: implied elements, implied end tags, table
fostering and the adoption agency algorithm MUST produce the tree the standard
defines, for well-formed and malformed markup alike.

#### Scenario: A row without a row group

- **WHEN** a `tr` element is the direct child of `table`
- **THEN** the tree holds a `tbody` element and the row belongs to it

#### Scenario: Content directly inside a table

- **WHEN** a paragraph appears between two rows of a table
- **THEN** the paragraph is placed before the table

#### Scenario: Markup inside a title element

- **WHEN** `title` contains `<b>text</b>`
- **THEN** the tree holds a single text node, not a `b` element

#### Scenario: An unclosed list item

- **WHEN** a `li` element is followed by another `li` without a closing tag
- **THEN** the two items are siblings in the list

### Requirement: Adjacent character data forms one text node

Character data that the standard tree leaves as consecutive sibling text nodes
SHALL be merged into one node, so a run of text between two elements is always a
single node whose value equals the concatenation in document order.

#### Scenario: Consecutive text nodes

- **WHEN** the tree construction leaves two text nodes next to each other in the
  same parent
- **THEN** the model holds one text node

#### Scenario: Text between inline elements

- **WHEN** a paragraph contains text, an inline element and more text
- **THEN** the first and last runs stay separate nodes, one on each side

### Requirement: White space between table structure takes no part in layout

Character data that consists only of white space and sits directly inside
`table`, `thead`, `tbody`, `tfoot`, `tr` or `colgroup` SHALL be ignored by
layout. It MUST NOT add a line box, a cell or spacing, and MUST NOT move the
table's own position.

#### Scenario: Indented table markup

- **WHEN** the rows of a table are separated by newlines and indentation
- **THEN** the table's layout equals the layout of the same table written on one
  line

#### Scenario: Text between rows

- **WHEN** a table contains only white space between a row group and a row
- **THEN** that white space produces no layout box

### Requirement: An incomplete tag at the end of the input is an error

A document whose input ends inside a start or end tag SHALL be rejected with an
`html/truncated` diagnostic positioned where the incomplete tag begins, and MUST
NOT convert a document shorter than the source. Input that ends between tokens,
including a trailing `<` that begins no tag, MUST parse as a complete document.

#### Scenario: The document ends inside a tag

- **WHEN** the input ends while a tag is still open, as in `<img src="`
- **THEN** the call fails with `html/truncated` naming the position of that tag

#### Scenario: The document ends between tokens

- **WHEN** the input ends after a closed element, after text, or with a trailing
  `<` that begins no tag
- **THEN** the document parses and converts normally

### Requirement: Every node carries a source position

Each element and text node SHALL carry the byte range it occupies in the source,
and a diagnostic reported for a node MUST name the file, line and column of that
range. A node that the tree construction creates for implied or reconstructed
markup SHALL inherit the range of its nearest ancestor that has one.

#### Scenario: An authored element

- **WHEN** an unsupported element appears on line 12 of the document
- **THEN** its diagnostic names line 12

#### Scenario: An implied element

- **WHEN** a diagnostic is reported for an element the parser implied
- **THEN** the diagnostic names the nearest authored ancestor's position and the
  element's path identifies the implied element

#### Scenario: Positions are stable

- **WHEN** the same document is validated twice
- **THEN** every diagnostic carries the same position

### Requirement: Parsing limits are enforced before the tree exists

The document SHALL be tokenized under the operation budget and the nesting limit
before the tree is constructed: a document over either limit MUST be rejected
with a `resource/limit` diagnostic, and a cancellation observed while tokens are
read MUST be returned as the context error, without the tree being allocated
first.

#### Scenario: Deep nesting is rejected early

- **WHEN** a document nests elements deeper than the configured limit
- **THEN** the call returns a `resource/limit` diagnostic

#### Scenario: Cancellation during parsing

- **WHEN** the context is cancelled while the document is read
- **THEN** the call returns the context error rather than a document diagnostic
