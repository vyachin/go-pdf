# Spec Delta

## MODIFIED Requirements

### Requirement: Parsing limits are enforced before the tree exists

The document SHALL be tokenized under the operation budget before the conversion model
is built: a document over that budget MUST be rejected with a `resource/limit`
diagnostic, and a cancellation observed while tokens are read MUST be returned as the
context error. Nesting depth is enforced as `Nesting depth is enforced on the standard
tree` specifies.

#### Scenario: Deep nesting is rejected early

- **WHEN** a document nests elements deeper than the configured limit
- **THEN** the call returns a `resource/limit` diagnostic

#### Scenario: Cancellation during parsing

- **WHEN** the context is cancelled while the document is read
- **THEN** the call returns the context error rather than a document diagnostic

## ADDED Requirements

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
