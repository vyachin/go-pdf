# html-parsing Specification

## Purpose

Defines how an HTML document becomes the document model the profile validates and
the layout engine lays out: how character references are decoded, how the tree is
constructed, which normalizations the profile relies on, and what source position
each node carries.

## Requirements

### Requirement: Input encoding is explicit or detected

The document encoding SHALL be the encoding the caller names. Without a name it SHALL be a byte order mark, then a `<meta charset>` or `<meta http-equiv="Content-Type">` declaration in the first 1024 bytes, and otherwise UTF-8. A UTF-8 byte order mark SHALL be removed before parsing.

#### Scenario: An explicit encoding

- **WHEN** the caller names windows-1251 for a document that declares nothing
- **THEN** the document is decoded as windows-1251 and its Cyrillic text converts

#### Scenario: A declared encoding

- **WHEN** a document declares windows-1251 in `<meta charset="windows-1251">`
- **THEN** the Cyrillic text decodes to the characters the author wrote

#### Scenario: A UTF-8 byte order mark

- **WHEN** a UTF-8 document starts with a byte order mark
- **THEN** the mark is removed, the document converts, and the output is byte identical to the same document without the mark

#### Scenario: A document that declares nothing

- **WHEN** a document declares no encoding and its bytes are valid UTF-8
- **THEN** it is parsed as UTF-8, as it was before this change

### Requirement: Only a closed set of input encodings is supported

The supported encodings SHALL be UTF-8, windows-1251, KOI8-R and IBM866. Any other declared or caller-named encoding SHALL be an error naming it, including UTF-16. A document-declared unsupported encoding MUST be `html/encoding`; a caller-named one MUST be `options/charset`.

#### Scenario: An unsupported declaration

- **WHEN** a document declares shift_jis
- **THEN** validation fails with `html/encoding` naming shift_jis and no PDF is produced

#### Scenario: A UTF-16 document

- **WHEN** a document starts with a UTF-16 byte order mark
- **THEN** validation fails with `html/encoding` naming UTF-16

#### Scenario: A caller names an unsupported encoding

- **WHEN** the caller names shift_jis
- **THEN** the request fails with `options/charset` before any file is read

#### Scenario: windows-1252 is not supported

- **WHEN** a document declares windows-1252
- **THEN** validation fails with `html/encoding` naming windows-1252 and listing the supported encodings

### Requirement: Undecodable input fails at the offending byte

A document that is not valid UTF-8 and declares no accepted encoding SHALL fail with `html/encoding` at the first invalid byte. A byte the selected encoding does not define SHALL fail with `html/encoding` at that byte. The engine MUST NOT replace either case with U+FFFD and continue.

#### Scenario: An undeclared legacy document

- **WHEN** a windows-1251 document with no declaration is validated
- **THEN** the error is `html/encoding` at the first invalid byte, not a glyph error

#### Scenario: An undefined byte

- **WHEN** a windows-1251 document contains byte 0x98, which the encoding does not define
- **THEN** the error is `html/encoding` at that byte

#### Scenario: The message names the way forward

- **WHEN** an undeclared document fails to decode
- **THEN** the message says that the document must declare a supported encoding or the caller must name one

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
markup SHALL inherit the range of its nearest ancestor that has one. When the
document is decoded from a legacy encoding, the range MUST still be a byte range
of the original source, so a position never moves because of decoding.

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

#### Scenario: A decoded document

- **WHEN** a windows-1251 document has Cyrillic text before an unsupported element
- **THEN** the diagnostic names the element's line and column in the original file

### Requirement: Parsing limits are enforced before the tree exists

The document SHALL be tokenized under the operation budget before the conversion
model is built: a document over that budget MUST be rejected with a
`resource/limit` diagnostic, and a cancellation observed while tokens are read
MUST be returned as the context error. Nesting depth is enforced as `Nesting
depth is enforced on the standard tree` specifies.

#### Scenario: Deep nesting is rejected early

- **WHEN** a document nests elements deeper than the configured limit
- **THEN** the call returns a `resource/limit` diagnostic

#### Scenario: Cancellation during parsing

- **WHEN** the context is cancelled while the document is read
- **THEN** the call returns the context error rather than a document diagnostic

### Requirement: Nesting depth is enforced on the standard tree

Element nesting SHALL be measured on the tree the HTML standard's construction
algorithm produces, after implied elements and implied end tags, and a document whose
deepest element exceeds the configured `MaxDOMDepth` MUST be rejected with a
`resource/limit` diagnostic before the conversion model or layout allocates.

#### Scenario: Implied end tags do not hide depth

- **WHEN** a document repeats `<p><div></p>` so that the standard tree is 23 elements
  deep and `MaxDOMDepth` is 8
- **THEN** the call returns a `resource/limit` diagnostic

#### Scenario: Implied end tags do not invent depth

- **WHEN** a document contains 300 sibling `<p>` elements with omitted end tags and
  `MaxDOMDepth` is 256
- **THEN** conversion proceeds and no `resource/limit` diagnostic is reported

#### Scenario: Implied elements are counted

- **WHEN** rows are direct children of `table`, so the standard tree adds a `tbody`
- **THEN** the measured depth includes the implied `tbody`

### Requirement: Proven over-limit nesting is rejected before the tree is built

Before the standard tree is constructed, the engine MAY reject a document when it can
prove the limit is exceeded without that tree: a run of start tags that can never be
implicitly closed, with no end tag and outside table or foreign content, MUST be
rejected with `resource/limit` once the run alone exceeds `MaxDOMDepth`. A document
that cannot be proven over the limit MUST NOT be rejected before the standard tree is
built.

#### Scenario: A large nested run fails early

- **WHEN** a document contains more never-implicitly-closed start tags in an unclosed
  chain than `MaxDOMDepth` allows
- **THEN** the call returns a `resource/limit` diagnostic without building the
  conversion model

#### Scenario: Ambiguous nesting is not rejected early

- **WHEN** the limit cannot be proven from the token stream because end tags or
  implicitly closable elements are present
- **THEN** the decision is made on the standard tree rather than on an approximation

### Requirement: Tokenizer names cannot corrupt the stack

The parser SHALL handle more distinct tag names than the default intern table holds: it
MUST grow the table or reject the document with a dedicated diagnostic. It MUST NOT
silently return a shared zero entry, because that appends a start tag to the wrong span
and prevents its end tag from popping the stack.

#### Scenario: A flat document with many tag names

- **WHEN** a flat document uses 70,000 distinct element names
- **THEN** it either parses or fails with a documented limit diagnostic
- **AND** it never reports a nesting error for a flat tree

#### Scenario: A tree-construction failure is coded

- **WHEN** the standard tree construction refuses a document with its internal "open
  stack exceeds 512 nodes" error
- **THEN** the call returns a `resource/limit` diagnostic with a position
- **AND** no raw third-party error escapes

### Requirement: An explicit charset strips a byte order mark

When the caller names an encoding, a UTF-8 byte order mark at the start of the document
SHALL be removed before decoding, exactly as it is when the encoding is detected. It
MUST NOT be decoded as visible text.

#### Scenario: Explicit windows-1251 with a BOM

- **WHEN** the caller names windows-1251 and the document starts with `EF BB BF`
- **THEN** the mark is removed and the text does not begin with "п»ї"

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
