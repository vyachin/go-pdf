# Spec Delta

## ADDED Requirements

### Requirement: Layout diagnostics preserve available source provenance

Layout SHALL retain available source provenance through splitting, shaping and
line construction. Attributable font, glyph and overflow failures MUST identify
the original file, nonzero source line/column and owning element. Generated text
MUST use its generating declaration when known. Positions MUST use the existing
original-byte mapper; normalization MUST NOT cause fabricated precision.

#### Scenario: A glyph is missing on a later line

- **WHEN** a multiline document contains a character the selected font cannot render inside a nested span
- **THEN** both entry points report `text/glyph` at the originating text or owning span with a nonzero source position and element path

#### Scenario: A margin-box glyph is missing

- **WHEN** a margin-box `content` declaration generates an unavailable character
- **THEN** the diagnostic identifies the stylesheet declaration that generated it when that source is available

#### Scenario: Decoded text retains its original source

- **WHEN** a supported legacy-encoded document fails during text layout after Cyrillic text
- **THEN** the diagnostic position refers to the original file through its existing byte mapper

#### Scenario: Normalization changes character boundaries

- **WHEN** entities or canonical composition prevent an exact glyph-to-byte mapping
- **THEN** the diagnostic uses the known owning source location and does not invent an exact character offset
