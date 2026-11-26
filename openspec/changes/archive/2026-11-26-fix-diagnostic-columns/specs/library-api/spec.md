# Spec Delta

## ADDED Requirements

### Requirement: A diagnostic column counts characters in every path (RV5-19)

Every diagnostic column derived from a document, a linked stylesheet or an encoding
error SHALL count Unicode characters from the start of the source line, not bytes. One
implementation MUST serve the document, mapper, stylesheet and charset paths. A leading
byte order mark SHALL be invisible to the column count, single-byte encodings SHALL keep
their current numbers, and a test MUST pin the rule for UTF-8 text, a BOM and a linked
stylesheet.

#### Scenario: A UTF-8 document with a byte order mark

- **WHEN** `<p>Привет</p><img src="missing.png">` is decoded from bytes that start with
  a UTF-8 byte order mark
- **THEN** the `img/load` diagnostic reports column 14
- **AND** the same document without the mark reports the same column

#### Scenario: A declaration in a linked UTF-8 stylesheet

- **WHEN** a linked UTF-8 stylesheet contains `/* Счёт */ .x { display: flex }`
- **THEN** the diagnostic reports the character column 26
- **AND** an equivalent inline rule and a legacy-encoded sheet report character columns
  under the same rule

#### Scenario: An encoding error after multibyte text

- **WHEN** an invalid byte follows a Cyrillic word on the second line
- **THEN** `html/encoding` reports the character column the author sees
- **AND** the column is not the byte offset

#### Scenario: Legacy encodings are unchanged

- **WHEN** a windows-1251 document reports a diagnostic
- **THEN** its column equals the byte column, because one byte is one character there
- **AND** existing windows-1251 position tests stay green
