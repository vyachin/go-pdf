# text-shaping Specification

## Purpose

Describes how characters of the input document become glyphs and lines: font loading, mapping code points to glyphs, kerning, line breaking, and the rules that Russian typography must not violate.

## Requirements

### Requirement: Font loading accepts local TrueType and OpenType fonts

The engine SHALL load fonts from local files named in `@font-face` or `Options.FallbackFont` in TrueType and OpenType formats. An OpenType font whose outlines are CFF SHALL be accepted through an in-memory conversion to TrueType. WOFF, WOFF2, CFF2, variable fonts and remote URLs MUST be rejected with a diagnostic naming the format or the source.

#### Scenario: A local TrueType font
- **WHEN** the template declares `@font-face` with a local PT Serif file in TrueType format
- **THEN** the font loads and is available for the document

#### Scenario: A local OpenType font with CFF outlines
- **WHEN** the template declares `@font-face` with a local `.otf` file whose outlines are CFF
- **THEN** the font loads, the text renders with its glyphs and the PDF embeds the glyphs the document uses

#### Scenario: A WOFF font
- **WHEN** the template declares `@font-face` with a local `.woff` or `.woff2` file
- **THEN** validation fails with a diagnostic naming the format

#### Scenario: A font by URL
- **WHEN** the template declares `@font-face` with `src: url(https://example.com/font.otf)`
- **THEN** validation fails: remote fonts are not supported

#### Scenario: A missing font file
- **WHEN** the path in `@font-face` points to a file that does not exist
- **THEN** a diagnostic reports the absolute path that could not be opened

### Requirement: CFF outlines convert within a tolerance, deterministically

A CFF glyph SHALL be converted to quadratic TrueType contours whose points stay within the tolerance recorded in `docs/fonts.md`, and the conversion MUST be deterministic: the same font bytes produce the same glyph data and the same PDF. The work and memory of a conversion SHALL be bounded; a font whose converted form exceeds the documented limit MUST fail with `resource/limit` instead of allocating without bound.

#### Scenario: Repeated conversion
- **WHEN** the same OpenType document is converted twice
- **THEN** the two PDFs are byte identical

#### Scenario: Outline fidelity
- **WHEN** a converted glyph is compared with the original CFF outline
- **THEN** every converted point lies within the recorded tolerance of the original curve

#### Scenario: The conversion limit
- **WHEN** a font converts to more than the documented limit
- **THEN** the conversion fails with `resource/limit` and no partial PDF is produced

### Requirement: Mapping code points to glyphs

The engine SHALL map characters to glyphs through a `cmap` table of format 4 or 12. If a character is missing from the selected font and from every alternative in the family, a diagnostic MUST name the character and its position in the document; silently substituting an empty glyph is not allowed.

#### Scenario: Cyrillic renders correctly
- **WHEN** the document contains the text "Счёт № 42 от 01.02.2026"
- **THEN** all characters, including "ё", "№" and "—", are rendered with existing glyphs

#### Scenario: Character missing from every candidate
- **WHEN** the text contains a character absent from the font family
- **THEN** rendering fails with a diagnostic naming the code point and its place in the document

### Requirement: Kerning from the GPOS table

The engine SHALL apply kerning pairs from the `GPOS` table when the font provides one. Without the table it MUST use widths from `hmtx` and report no error.

#### Scenario: A kerning pair is applied
- **WHEN** the font contains a kerning pair for adjacent characters and the document uses it
- **THEN** the distance between the glyphs differs from the sum of their `hmtx` widths

#### Scenario: Font without GPOS
- **WHEN** the font has no `GPOS` table
- **THEN** text is rendered with `hmtx` widths and no diagnostics

### Requirement: Malformed optional font tables do not crash parsing

Parsing a font with a truncated or inconsistent optional table SHALL NOT panic. Every field read MUST be preceded by a bounds check for that table format. A corrupt optional table MAY be ignored, in which case the font loads without the feature it provides; a corrupt required table MUST produce an error.

#### Scenario: Truncated class definition
- **WHEN** a class definition in a GPOS table ends before its range header
- **THEN** parsing does not panic
- **AND** the font either loads without kerning or returns an error

#### Scenario: Truncated kerning table
- **WHEN** the GPOS table is truncated in the middle of a pair set
- **THEN** parsing does not panic
- **AND** the font loads with no kerning pairs

#### Scenario: Required table missing
- **WHEN** a required table such as `glyf` is absent
- **THEN** parsing returns an error naming the missing table

### Requirement: Font parsing work is bounded

Font parsing SHALL charge the shared work guard for every lookup, subtable and coverage range it examines, and SHALL stop with a `resource/limit` diagnostic when the configured budget is exhausted. The number of entries a font declares MUST NOT let a small file bypass the budget, and parsing MUST observe context cancellation.

#### Scenario: The amplifier font stops at the budget
- **WHEN** a font whose GPOS table repeats one PairPos across 40,000 subtables is
  loaded with a work budget that the parse would exceed
- **THEN** the call returns `resource/limit` instead of parsing every subtable

#### Scenario: A normal font is unaffected
- **WHEN** a regular text font is parsed under the default limits
- **THEN** the font loads with its kerning pairs and no diagnostics

#### Scenario: Cancellation during parsing
- **WHEN** the context is cancelled while a large font is parsed
- **THEN** the call returns the context error

### Requirement: Font-declared sizes never overflow

Every offset, length and count read from a font SHALL be evaluated in 64-bit arithmetic before it is converted or used to slice, on every supported architecture. A value outside its table or the glyph-id range MUST be rejected, and a `cmap` format 12 group whose `startGlyphID + count` exceeds 0xFFFF MUST be rejected.

#### Scenario: A high offset on a 32-bit build
- **WHEN** a font declares a `cmap` offset of 0x80000000 on a 32-bit build
- **THEN** the font is rejected with a parse error and the process does not panic

#### Scenario: Format 12 glyph id overflow
- **WHEN** a format 12 group starts at `startGlyphID: 0xFFFFFFF0` and spans several
  characters
- **THEN** parsing rejects the group instead of mapping the characters to low glyphs

### Requirement: Non-breaking sequences are not split

The engine SHALL treat a no-break space (U+00A0) as a prohibition of a line break at that position and MUST keep it as a word separator when justifying.

#### Scenario: No-break space in an address
- **WHEN** the document contains "г." + U+00A0 + "Москва"
- **THEN** the pair stays on one line for every available break

#### Scenario: No-break space under justification
- **WHEN** a line containing a no-break space is justified
- **THEN** the gap at the no-break space is not stretched

### Requirement: Line breaking

The engine SHALL break lines at spaces and after a hyphen without splitting a word that has no hyphen, and MUST keep trailing punctuation with the preceding word. Hyphenation (`hyphens`) is not supported in v1 and MUST be rejected as an unsupported property.

#### Scenario: A long word wider than the line
- **WHEN** a word without spaces does not fit the available width
- **THEN** it moves to the next line as a whole instead of being split in the middle

#### Scenario: Requesting automatic hyphenation
- **WHEN** the template contains `hyphens: auto`
- **THEN** validation fails and states that hyphenation is outside the v1 profile

### Requirement: Justification

With `text-align: justify` the engine SHALL distribute extra space between word separators. The last line of a paragraph and the line before a forced break MUST remain unstretched.

#### Scenario: A paragraph of several lines
- **WHEN** a paragraph occupies three lines and is justified
- **THEN** the first two lines have the same width, equal to the available width, and the third stays unstretched

#### Scenario: A line before a page break
- **WHEN** a line ends the page and the next one starts a new page
- **THEN** the closing line is not stretched

### Requirement: Composition of combining characters

The engine SHALL normalise input text to canonical composition before mapping it to glyphs, so that "й" and "ё" written in decomposed form render as a single glyph rather than a base character plus a separate diacritic.

#### Scenario: Decomposed "й"
- **WHEN** the text contains "и" + U+0306
- **THEN** the output uses the "й" glyph from the font
- **AND** text extracted from the PDF contains a single "й" character

#### Scenario: Canonically composed text is unchanged
- **WHEN** the text is already written in canonical composition
- **THEN** mapping to glyphs produces the same result
