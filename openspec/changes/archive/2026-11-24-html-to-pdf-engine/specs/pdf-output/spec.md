# Spec Delta

## Purpose

Defines the requirements for the finished PDF file: validity of the structure, embedding of fonts and the text layer, images, metadata, bookmarks and byte-for-byte reproducibility of the result.

## ADDED Requirements

### Requirement: Valid PDF structure

Every created file MUST conform to the PDF specification and MUST pass independent structure validation. The file MUST contain a correct cross-reference table, catalog dictionary, page tree and the resources the pages reference.

#### Scenario: The file passes independent validation
- **WHEN** a generated PDF is checked by an independent structure validator
- **THEN** no structure errors are found

#### Scenario: A single-page document
- **WHEN** the document spans exactly one page
- **THEN** the PDF contains one page with correct dimensions and resources

### Requirement: Embedding font subsets

The engine SHALL embed in the PDF only those glyphs of a font that are used in the document, and SHALL mark the font as embedded. The document MUST render identically on a machine without the original font installed.

#### Scenario: The subset contains the used glyphs
- **WHEN** a document uses 30 different characters of a font that contains 600 glyphs
- **THEN** a subset covering those 30 characters is embedded in the PDF

#### Scenario: Rendering without the font installed
- **WHEN** the PDF is opened on a machine where the original font is not installed
- **THEN** the text is rendered in the same style as during generation

### Requirement: Copyable and searchable text layer

The engine SHALL include a `ToUnicode` map for every embedded font in the PDF, so that text is extracted, copied and found by search in its original form, including Cyrillic.

#### Scenario: Extraction of Russian text
- **WHEN** text is extracted from the generated PDF
- **THEN** it contains «Счёт № 42 от 01.02.2026» in the original form

#### Scenario: Search through the document
- **WHEN** a search for the word «договор» is run in a PDF viewer
- **THEN** the word is found in the text of the document

### Requirement: Images without quality loss

The engine SHALL embed JPEG without recompression and PNG with decoding into a stream without loss of pixel data; lossy recompression is not acceptable.

#### Scenario: JPEG is embedded as is
- **WHEN** the document contains a JPEG image
- **THEN** the original compressed data of the image lands in the PDF

#### Scenario: PNG transparency
- **WHEN** a PNG contains an alpha channel
- **THEN** transparency is either reproduced correctly, or validation reports the limitation of profile v1

### Requirement: Document metadata

The engine SHALL fill the PDF title from `<title>` and SHALL carry over `author`, `description` and `keywords` from the document meta tags when they are set.

#### Scenario: Title from title
- **WHEN** the document contains `<title>Счёт № 42</title>`
- **THEN** in the PDF properties the title equals «Счёт № 42»

#### Scenario: Metadata not set
- **WHEN** the document contains no author and description meta tags
- **THEN** the corresponding fields stay empty, without an error

### Requirement: Reproducible output

With the same input document, the same run parameters and the same fonts, the engine SHALL create byte-identical PDF files. The document identifier and the creation date MUST be set deterministically, and not from the system clock.

#### Scenario: Two runs give the same file
- **WHEN** the same template is rendered twice
- **THEN** the checksums of both files match

#### Scenario: An explicitly set creation date
- **WHEN** the run sets the creation date of the document
- **THEN** that date lands in the PDF metadata, and not the current system time

### Requirement: Document bookmarks

The engine SHALL create a bookmark tree from the `h1`–`h3` headings in order of appearance, nested by heading level and bound to the page the heading is on.

#### Scenario: Nested bookmarks
- **WHEN** the document contains an `h1`, then two `h2`
- **THEN** in the bookmark tree the `h1` is the root node, and the `h2` are nested in it

#### Scenario: A bookmark leads to the right page
- **WHEN** an `h2` heading is on page 5
- **THEN** the corresponding bookmark opens page 5

### Requirement: Atomic write of the result

The engine SHALL write the result so that on any rendering error no partially written or corrupt file is left at the destination.

#### Scenario: An error at the write stage
- **WHEN** an error occurs in the process of creating the PDF
- **THEN** the target file is absent or stays as it was, but does not become corrupt
