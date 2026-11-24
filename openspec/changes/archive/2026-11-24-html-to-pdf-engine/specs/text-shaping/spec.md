# Spec Delta

## Purpose

Describes how the characters of the input document turn into glyphs and lines: font loading, mapping of code points, kerning, line breaking and the rules that must not be violated in Russian typography.

## ADDED Requirements

### Requirement: Font loading in TrueType format only

The engine SHALL load fonts from local files named in `@font-face`, and SHALL support only the TrueType format (`.ttf`). CFF/OpenType, WOFF and WOFF2 fonts as well as references to remote fonts MUST be rejected with a diagnostic naming the format or the source.

#### Scenario: A local TrueType font
- **WHEN** the template declares `@font-face` with a local PT Serif file in TrueType format
- **THEN** the font is loaded and available for use in the document

#### Scenario: A font by URL
- **WHEN** the template declares `@font-face` with `src: url(https://example.com/font.woff2)`
- **THEN** validation fails: remote and non-TrueType fonts are not supported

#### Scenario: A missing font file
- **WHEN** the path in `@font-face` points to a file that does not exist
- **THEN** a diagnostic is emitted with the absolute path that could not be opened

### Requirement: Mapping code points to glyphs

The engine SHALL map characters to glyphs through the `cmap` table of format 4 or 12. If a character is missing in the selected font and in none of the alternative fonts of the family, a diagnostic MUST be emitted naming the character and its position in the document; a silent substitution with an empty glyph is not acceptable.

#### Scenario: Cyrillic is mapped correctly
- **WHEN** the document contains the text «Счёт № 42 от 01.02.2026»
- **THEN** all characters, including «ё», «№» and «—», are mapped to existing glyphs

#### Scenario: A character missing in all candidates
- **WHEN** the text contains a character missing from the font family
- **THEN** rendering fails, naming the code point and the place in the document

### Requirement: Kerning from the GPOS table

The engine SHALL apply kerning pairs from the `GPOS` table when the font has it. When the table is absent, the widths from `hmtx` MUST be used without kerning, without an error.

#### Scenario: A kerning pair is applied
- **WHEN** the font contains a kerning pair for adjacent characters and the document uses it
- **THEN** the distance between the glyphs differs from the sum of their widths from `hmtx`

#### Scenario: A font without GPOS
- **WHEN** the font contains no `GPOS` table
- **THEN** the text is rendered with the widths from `hmtx` without a diagnostic

### Requirement: Non-breaking constructs do not break

The engine SHALL treat the non-breaking space (U+00A0) as a prohibition of a line break at that position and SHALL preserve the non-breaking space as an inter-word interval when justifying.

#### Scenario: A non-breaking space in an address
- **WHEN** the document contains «г.» + U+00A0 + «Москва»
- **THEN** this pair stays on one line under any available breaking

#### Scenario: A non-breaking space under justification
- **WHEN** a line with a non-breaking space is justified
- **THEN** the interval at the position of the non-breaking space is not stretched

### Requirement: Line breaking

The engine SHALL break a line at spaces and after a hyphen, without breaking a word that has no hyphen, and SHALL move hanging punctuation together with the preceding word. Hyphenation by syllables (`hyphens`) is not supported in v1 and MUST be rejected as an unsupported property.

#### Scenario: A long word wider than the line
- **WHEN** a word without spaces does not fit the available width
- **THEN** it is carried to the next line as a whole, and not broken in the middle

#### Scenario: A request for automatic hyphenation
- **WHEN** the template contains `hyphens: auto`
- **THEN** validation fails, stating that hyphenation is not part of the v1 profile

### Requirement: Justification

With `text-align: justify` the engine SHALL distribute the additional space between inter-word spaces. The last line of a paragraph and the line before a forced break MUST stay unstretched.

#### Scenario: A paragraph of several lines
- **WHEN** a paragraph spans three lines and is justified
- **THEN** the first two lines have the same width, equal to the available one, and the third stays unstretched

#### Scenario: A line before a page break
- **WHEN** a line ends a page and the next one starts on a new page
- **THEN** the finishing line is not stretched by justification

### Requirement: Normalisation of composite characters

The engine SHALL reduce the input text to the canonical composition before mapping to glyphs, so that «й» and «ё» written decomposed render as one glyph, and not as a base character with a separate diacritic.

#### Scenario: A decomposed «й»
- **WHEN** the text contains «и» + U+0306
- **THEN** the output uses the «й» glyph from the font
- **AND** the text extracted from the PDF contains one character «й»

#### Scenario: Canonically written text does not change
- **WHEN** the text is already written in canonical composition
- **THEN** the mapping to glyphs gives the same result
