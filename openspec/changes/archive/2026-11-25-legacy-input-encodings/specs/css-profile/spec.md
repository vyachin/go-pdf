# Spec Delta

## ADDED Requirements

### Requirement: External stylesheet encoding

An external stylesheet SHALL be decoded with the document's encoding. A `@charset` rule at the start of the file SHALL be accepted when it names the same encoding, and MUST fail with `css/link-encoding` when it names another. A byte the encoding does not define MUST fail with `css/link-encoding` at that byte. A diagnostic for a rule in the file MUST name its position in the original file.

#### Scenario: A matching charset rule
- **WHEN** the document is windows-1251 and `print.css` starts with `@charset "windows-1251";`
- **THEN** the sheet applies and there are no diagnostics

#### Scenario: A conflicting charset rule
- **WHEN** the document is windows-1251 and `print.css` starts with `@charset "utf-8";`
- **THEN** validation fails with `css/link-encoding` naming both encodings

#### Scenario: An undefined byte in a sheet
- **WHEN** a windows-1251 stylesheet contains byte 0x98, which the encoding does not define
- **THEN** validation fails with `css/link-encoding` at that byte

#### Scenario: A position inside a decoded sheet
- **WHEN** a Cyrillic rule precedes an unsupported declaration in a windows-1251 stylesheet
- **THEN** the diagnostic names the declaration's line and column in the original file, not in the decoded text
