# Spec Delta

## ADDED Requirements

### Requirement: Margin boxes report shaping failures

When the text of a margin box cannot be shaped because a character is missing from the font, the engine SHALL emit an error diagnostic `text/glyph` naming the margin box and MUST NOT produce a document that silently omits the header or footer.

#### Scenario: Missing glyph in a footer
- **WHEN** `@bottom-center` contains a character absent from the selected font
- **THEN** conversion fails with a `text/glyph` diagnostic naming the box

#### Scenario: Ordinary footer
- **WHEN** the footer text uses characters the font has
- **THEN** it is rendered on every page as before

### Requirement: Block images are placed as replaced blocks

An `img` with `display: block` SHALL be placed as a replaced block: resolved with the same sizing rules as an inline image, painted at the content-box origin, and advanced in the flow like any other block.

#### Scenario: Image between paragraphs
- **WHEN** the document contains a paragraph, a block image and another paragraph
- **THEN** the page contains an image operation
- **AND** both paragraphs are rendered around it

#### Scenario: Image only document
- **WHEN** the body consists of one block image
- **THEN** it is painted once
