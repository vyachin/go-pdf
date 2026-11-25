# Spec Delta

## ADDED Requirements

### Requirement: Generated text preserves shaped glyph offsets

Every painted shaped run, including headers, footers and markers, SHALL preserve
its horizontal and vertical glyph offsets until painting. Equivalent runs using
the same font and size MUST have equivalent relative glyph positions in body and
generated text. Unsupported mark attachment MUST remain a diagnostic rather than
an incorrectly positioned glyph.

#### Scenario: A stressed word in a margin box

- **WHEN** body text and a page margin box render `сло́во` with the same face and size
- **THEN** the mark has the same position relative to its base in both locations
- **AND** extracted text preserves each original combining sequence

#### Scenario: A mark has a vertical offset

- **WHEN** a generated text run contains a mark with nonzero vertical attachment
- **THEN** the mark is painted at the shaper's horizontal and vertical offsets

#### Scenario: Plain runs keep their output

- **WHEN** a run contains no positioned marks
- **THEN** its advances, alignment and text extraction remain unchanged
