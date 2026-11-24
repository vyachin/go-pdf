# pdf-output Specification

## Purpose

Defines the requirements for the finished PDF file: valid structure, embedded fonts and a searchable text layer, images, metadata, bookmarks and byte-for-byte reproducibility.

## Requirements

### Requirement: Valid PDF structure

Every produced file SHALL conform to the PDF specification and MUST pass independent structural validation. The file MUST contain a correct cross-reference table, catalog dictionary, page tree and the resources its pages reference.

#### Scenario: The file passes independent validation
- **WHEN** the generated PDF is checked by an independent structural validator
- **THEN** no structural errors are reported

#### Scenario: Single page document
- **WHEN** the document occupies exactly one page
- **THEN** the PDF contains one page with the correct size and resources

### Requirement: Font objects form a complete reference chain

Every embedded font SHALL be written as five distinct objects: Type0, CIDFontType2, FontDescriptor, the FontFile2 stream and the ToUnicode stream. `/DescendantFonts` MUST reference the CIDFontType2 object, its `/FontDescriptor` entry MUST reference the descriptor, and the descriptor MUST reference the font file stream. No object identifier may be written twice for different object types.

#### Scenario: Descendant font reference
- **WHEN** a PDF is produced from a template containing text
- **THEN** following `/DescendantFonts` from the Type0 font leads to an object with `/Subtype /CIDFontType2`
- **AND** that object is not a `/FontDescriptor`

#### Scenario: Descriptor and font file
- **WHEN** the CIDFontType2 object is followed through `/FontDescriptor`
- **THEN** the descriptor exists
- **AND** its `/FontFile2` entry references a stream object, not another dictionary

#### Scenario: Text remains extractable
- **WHEN** a document with Cyrillic text is produced after the fix
- **THEN** text extraction returns the original characters

#### Scenario: Independent validation
- **WHEN** the generated PDF is checked by an independent validator that resolves object references
- **THEN** no structural error is reported

### Requirement: Font subset embedding

The engine SHALL embed only the glyphs of a font that the document actually uses and MUST mark the font as embedded. The document MUST render identically on a machine that does not have the source font installed.

#### Scenario: The subset covers used glyphs
- **WHEN** a document uses 30 distinct characters of a font that contains 600 glyphs
- **THEN** the PDF embeds a subset covering those 30 characters

#### Scenario: Rendering without the installed font
- **WHEN** the PDF is opened on a machine where the source font is not installed
- **THEN** the text is rendered in the same typeface as during generation

### Requirement: Copyable and searchable text layer

The engine SHALL include a `ToUnicode` map for every embedded font so that text can be extracted, copied and found in its original form, including Cyrillic.

#### Scenario: Russian text extraction
- **WHEN** text is extracted from the generated PDF
- **THEN** it contains "Счёт № 42 от 01.02.2026" as written

#### Scenario: Search inside the document
- **WHEN** a PDF viewer searches for the word "договор"
- **THEN** the word is found in the document text

### Requirement: Images without quality loss

The engine SHALL embed JPEG without recompression and PNG by decoding into a stream without losing pixel data; lossy recompression is not allowed.

#### Scenario: JPEG is embedded as is
- **WHEN** the document contains a JPEG image
- **THEN** the PDF carries the original compressed image data

#### Scenario: PNG transparency
- **WHEN** a PNG contains an alpha channel
- **THEN** transparency is either reproduced correctly, or validation reports the v1 profile limitation

### Requirement: Document metadata

The engine SHALL fill the PDF title from `<title>` and MUST copy `author`, `description` and `keywords` from the document meta tags when they are present.

#### Scenario: Title from the title element
- **WHEN** the document contains `<title>Счёт № 42</title>`
- **THEN** the PDF title property equals "Счёт № 42"

#### Scenario: Metadata not provided
- **WHEN** the document has no author or description meta tags
- **THEN** the corresponding fields stay empty, without an error

### Requirement: Reproducible output

For the same input document, the same run options and the same fonts, the engine SHALL produce byte-identical PDF files. The document identifier and creation date MUST be deterministic rather than taken from the system clock.

#### Scenario: Two runs produce the same file
- **WHEN** the same template is rendered twice
- **THEN** the checksums of both files match

#### Scenario: Explicit creation date
- **WHEN** the run specifies a document creation date
- **THEN** that date is stored in the PDF metadata instead of the current system time

### Requirement: Document bookmarks

The engine SHALL build a bookmark tree from `h1`–`h3` headings in document order, nested by heading level and anchored to the page where the heading appears.

#### Scenario: Nested bookmarks
- **WHEN** the document contains an `h1` followed by two `h2` headings
- **THEN** the bookmark tree has the `h1` as the root node with the `h2` entries nested under it

#### Scenario: A bookmark opens the right page
- **WHEN** an `h2` heading appears on page 5
- **THEN** its bookmark opens page 5

### Requirement: Atomic result writing

The engine SHALL write the result so that after any rendering error the destination holds either no file or the previous file, never a partially written or corrupted one.

#### Scenario: Error during writing
- **WHEN** an error occurs while the PDF is being produced
- **THEN** the target file is absent or unchanged, and never corrupted
