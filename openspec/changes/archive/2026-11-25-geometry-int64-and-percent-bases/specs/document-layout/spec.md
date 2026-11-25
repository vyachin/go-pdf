# Spec Delta

## ADDED Requirements

### Requirement: Layout geometry does not overflow

Every layout length SHALL be computed in 64-bit arithmetic and stored in a 32-bit
field only after a range check. An unrepresentable value MUST be clamped or rejected
with a diagnostic, never wrapped; a negative image or box dimension MUST NOT reach the
PDF. A declared `width: 0` SHALL produce a zero-width box.

#### Scenario: A very wide image

- **WHEN** a document uses a 3,000,000×2 PNG under the pixel limit
- **THEN** the image's intrinsic width never becomes negative
- **AND** the content stream contains no negative image scale

#### Scenario: Very large padding

- **WHEN** nested blocks declare `padding-left: 2000000pt` twice
- **THEN** text is not painted at a negative X coordinate without a diagnostic

#### Scenario: Zero width

- **WHEN** a block declares `width: 0` with a background colour
- **THEN** the paint operation has zero width and no visible fill
