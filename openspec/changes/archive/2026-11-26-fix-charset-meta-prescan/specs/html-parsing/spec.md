# Spec Delta

## ADDED Requirements

### Requirement: The encoding prescan reads real meta declarations only (RV5-20)

The prescan over the first 1024 bytes SHALL ignore comments, doctypes, processing
instructions, non-`meta` tags and attribute values that are not a declaration. It MUST
recognise only a real `charset` attribute or an `http-equiv="content-type"` element whose
`content` names a charset. A declaration found in a comment MUST NOT influence detection,
and an ignored tag MUST NOT be named in an `html/encoding` message.

#### Scenario: A declaration inside a comment

- **WHEN** a windows-1251 document starts with `<!-- <meta charset="utf-8"> -->` and then
  contains invalid UTF-8 bytes
- **THEN** the prescan finds no declaration
- **AND** `html/encoding` reports the document as undecodable at the first invalid byte,
  never as "unsupported document encoding utf-8"

#### Scenario: The word charset inside another attribute

- **WHEN** a document contains `<meta name="description" content="charset is a word">`
  followed by windows-1251 bytes
- **THEN** the prescan finds no declaration
- **AND** no diagnostic quotes `is` as an encoding name

#### Scenario: A genuine charset declaration

- **WHEN** a document contains `<meta charset="windows-1252">`
- **THEN** the prescan reports windows-1252
- **AND** validation fails with `html/encoding` naming windows-1252

#### Scenario: A genuine http-equiv declaration

- **WHEN** a document contains
  `<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`
- **THEN** the prescan reports windows-1251
- **AND** the document decodes as windows-1251

#### Scenario: A byte order mark wins over a declaration

- **WHEN** a UTF-8 document with a byte order mark also carries a windows-1251 meta
  declaration in its first 1024 bytes
- **THEN** the mark decides the encoding
- **AND** the declaration is not reported as a conflict
