# Spec Delta

## ADDED Requirements

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
