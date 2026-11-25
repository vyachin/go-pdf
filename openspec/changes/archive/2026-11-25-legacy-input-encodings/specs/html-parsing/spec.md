# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

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
