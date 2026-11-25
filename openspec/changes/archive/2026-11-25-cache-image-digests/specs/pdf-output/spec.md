# Spec Delta

## ADDED Requirements

### Requirement: Image content digests are reused and budgeted

The engine SHALL compute an image content digest at most once per loaded image
identity in one conversion. Digest work MUST be proportional to unique encoded
bytes, charged to the shared work guard before processing bounded chunks, and
cancellable. A repeated placement or layout pass MUST reuse the cached digest.
Distinct files with identical content MUST still share one PDF image object.

#### Scenario: A large image has many placements

- **WHEN** a 3.76 MB JPEG is placed 1,000 times, including on different pages
- **THEN** a deterministic hashing counter records one pass over that loaded image's bytes
- **AND** the PDF contains one image object and preserves first-use resource order

#### Scenario: Hashing exceeds the remaining budget

- **WHEN** the remaining work budget cannot cover the first image digest
- **THEN** conversion reports `resource/limit` before hashing the bytes beyond the budget

#### Scenario: Cancellation during hashing

- **WHEN** the context is cancelled while unique image bytes are hashed
- **THEN** conversion returns the context error before hashing all remaining placements
