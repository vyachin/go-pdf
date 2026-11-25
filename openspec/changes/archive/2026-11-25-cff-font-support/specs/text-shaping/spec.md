# Spec Delta

## REMOVED Requirements

### Requirement: Font loading is limited to TrueType

**Reason**: OpenType fonts with CFF outlines are accepted through an in-memory conversion to TrueType; the requirement only made sense while the engine had no CFF interpreter.

**Migration**: none for the author — a document that previously failed now converts. WOFF, WOFF2, CFF2 and remote sources stay rejected, with the same diagnostic shape.

## ADDED Requirements

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
