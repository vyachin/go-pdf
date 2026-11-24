# Spec Delta

## ADDED Requirements

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
