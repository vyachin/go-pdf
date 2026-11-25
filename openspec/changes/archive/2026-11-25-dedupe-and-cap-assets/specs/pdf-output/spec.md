# Spec Delta

## MODIFIED Requirements

### Requirement: A repeated image is embedded once

When two references resolve to the same image file, the engine SHALL decode it once
and embed a single image object. Identity SHALL be the canonical resolved path, or
the content hash when no path exists: different spellings of one path
(`a.png`, `./a.png`, `sub/../a.png`) and repeated `data:` URIs MUST count as one
source.

#### Scenario: The same image twice

- **WHEN** a document references the same image file in two places
- **THEN** the PDF contains one image object for it
- **AND** both places draw that object

#### Scenario: Two spellings of one file

- **WHEN** a document references one file as `a.png` and `./a.png`
- **THEN** the PDF contains one image object for it

#### Scenario: The same data URI twice

- **WHEN** one `data:image/png` URI is used by an `<img>` and a background
- **THEN** the PDF contains one image object for it

#### Scenario: Different files stay distinct

- **WHEN** two files contain different bytes but share a base name in different
  directories
- **THEN** the PDF contains one image object per file
