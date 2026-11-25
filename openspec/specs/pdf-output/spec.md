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

The package SHALL NOT write the result: it returns the PDF to the caller. The reference command SHALL write the file only after a complete PDF was returned, so that a conversion that fails leaves the destination absent or unchanged and never partially written.

#### Scenario: Error during writing
- **WHEN** a conversion fails and the output path already holds a file
- **THEN** the target file is byte-identical to what it was before the run

#### Scenario: A successful conversion replaces the destination
- **WHEN** a conversion succeeds and the output path already holds a file
- **THEN** the destination holds the new PDF and no other file is created

### Requirement: Page resources list only images the page draws

A page's resource dictionary SHALL list only the images that page actually draws. Images used on other pages MUST NOT appear in it. The order of the listed images MUST remain deterministic.

#### Scenario: An image used on one page
- **WHEN** a document of two pages draws an image only on the first page
- **THEN** the second page's resources contain no image entries

#### Scenario: Two images on the same page
- **WHEN** a page draws two different images
- **THEN** both appear in that page's resources in document order

#### Scenario: Output stays deterministic
- **WHEN** the same document is serialised repeatedly
- **THEN** every run produces identical bytes

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

### Requirement: Deterministic image resource ordering

Image resources SHALL be written in a deterministic order derived from the order in which images first appear in the document. Repeated serialisation of the same document MUST produce byte-identical output, including the document identifier.

#### Scenario: Repeated serialisation with several images
- **WHEN** a document containing three images is serialised thirty times
- **THEN** every serialisation produces the same bytes

#### Scenario: Image order follows the document
- **WHEN** a document places image A before image B
- **THEN** the resource entries list A before B

### Requirement: Glyph positions match layout including kerning

The writer SHALL use the advance calculated by layout for every glyph and MUST express the difference from the nominal font width as a `TJ` adjustment. Positions in the PDF MUST match layout for runs with nonzero kerning, and the text layer MUST keep extracting the same characters.

#### Scenario: A kerned pair is preserved
- **WHEN** a run contains adjacent glyphs with nonzero kerning in the font
- **THEN** the content stream carries a nonzero `TJ` adjustment between them

#### Scenario: Extraction still works
- **WHEN** text is extracted from a PDF whose runs are written as `TJ` arrays
- **THEN** the extracted characters match the document text

#### Scenario: No kerning needed
- **WHEN** every advance equals the nominal width
- **THEN** no adjustment numbers are written and the run still renders correctly

### Requirement: Data URI image sources

An `<img src>` and a `background-image` value SHALL accept a base64-encoded `data:image/png` or `data:image/jpeg` URI. The decoded bytes MUST pass through the same pixel and byte budgets as an image file, and a repeated URI SHALL be decoded once and embedded once, exactly like a repeated file. Any other data URI type, malformed base64 or a non-image data URI SHALL be rejected with a diagnostic naming the problem.

#### Scenario: An image embedded in the document
- **WHEN** a document contains `<img src="data:image/png;base64,…">`
- **THEN** the image is embedded in the PDF and the text layer is unaffected

#### Scenario: A background image in a data URI
- **WHEN** a box declares `background-image: url("data:image/jpeg;base64,…")`
- **THEN** the decoded JPEG is painted behind the box's content

#### Scenario: The same data URI twice
- **WHEN** the same data URI is used by an `<img>` and a background
- **THEN** the PDF contains one image object for it

#### Scenario: A vector data URI
- **WHEN** the source is `data:image/svg+xml;base64,…`
- **THEN** conversion fails with a diagnostic saying that only PNG and JPEG are decoded

#### Scenario: Malformed base64
- **WHEN** the payload is not valid base64
- **THEN** conversion fails with a diagnostic naming the malformed source

### Requirement: Independent validation in the test pipeline

The test pipeline SHALL validate produced PDFs with an independent validator located by `exec.LookPath`, with an explicit `PDFCPU` override. When `PDF_REQUIRE_VALIDATOR=1` is set and no validator is found, the tests MUST fail; otherwise the skip MUST be reported. Continuous integration MUST run the pipeline with the validator installed and the variable set.

#### Scenario: Validator available
- **WHEN** the tests run with pdfcpu in `PATH` or `PDFCPU` set
- **THEN** every produced PDF is validated by that tool

#### Scenario: Validator required but missing
- **WHEN** `PDF_REQUIRE_VALIDATOR=1` is set and no validator is found
- **THEN** the test fails with a message naming the variable

#### Scenario: Local run without the tool
- **WHEN** the variable is unset and no validator is found
- **THEN** the test passes and logs that independent validation was skipped

### Requirement: Font descriptors describe the embedded font

An embedded subset SHALL be marked with the six-uppercase-letter subset tag and a
`+` prefix in both `/BaseFont` and `/FontName`. `/CapHeight` MUST come from the
font's `OS/2` `sCapHeight` (or a measured capital when absent), and `/ItalicAngle`
MUST come from the font's `post` table, so an italic face does not report 0.

#### Scenario: The subset is named as a subset

- **WHEN** a two-page document is written and inspected with `pdffonts`
- **THEN** the font is reported as embedded and subset

#### Scenario: An italic face

- **WHEN** the document uses an italic face
- **THEN** `/ItalicAngle` is the font's angle, not 0

#### Scenario: Cap height

- **WHEN** the font declares `sCapHeight` 1493 at 2048 units per em
- **THEN** `/CapHeight` is 729, not the ascent

### Requirement: Metadata strings are valid Unicode

The engine SHALL reject a metadata value that is not valid UTF-8 with a diagnostic
instead of replacing bytes with U+FFFD. Every `/Title`, `/Author`, `/Subject` and
`/Keywords` string MUST round-trip through `pdfinfo` unchanged.

#### Scenario: An invalid title

- **WHEN** `Options.Title` contains `\xff`
- **THEN** the call returns `options/title` and no PDF

#### Scenario: A valid title round-trips

- **WHEN** the title contains parentheses, a backslash and an emoji
- **THEN** `pdfinfo` prints exactly the input string

### Requirement: Font resources are validated

The writer SHALL return an error when a text operation has no font and SHALL reject a
font that needs more than 65,535 CIDs with a `resource/limit` diagnostic instead of
wrapping the `uint16` code.

#### Scenario: A nil font

- **WHEN** a document operation carries a nil font
- **THEN** `Bytes` returns an error and does not panic

#### Scenario: Too many CID pairs

- **WHEN** one font is used with more than 65,535 distinct glyph-rune pairs
- **THEN** the conversion fails with `resource/limit`

### Requirement: Outline levels are normalised

An outline entry with a level below 1 SHALL be treated as level 1 when the tree is
built, including in the comparison that decides nesting.

#### Scenario: A zero level

- **WHEN** entries are `{A, level 0}` and `{B, level 1}`
- **THEN** B is a sibling of A, not its child

### Requirement: The debug raster honours the image clip

`Result.PNG` SHALL apply the same clip as the PDF for a background image: no pixel
outside the box may carry the image, and no other operation may be clipped.

#### Scenario: A covered background

- **WHEN** a `cover` background is wider than its box and `RenderPNG` is set
- **THEN** pixels outside the box are the page background
- **AND** the PDF renders identically
