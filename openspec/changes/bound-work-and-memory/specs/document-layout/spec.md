# Spec Delta

## ADDED Requirements

### Requirement: Text placement reuses and charges shaped glyphs (RV5-14)

Placing a shaped run SHALL NOT build a second glyph slice when no glyph needs an
offset. When a copy is required it MUST be allocated at exact size and SHALL be
charged one unit per glyph before the allocation. A single 8 MiB word under the
default limits MUST stay within the documented allocation multiple of its rune count,
and a small `MaxOperations` MUST return `resource/limit` before the copies are made.
Output for ordinary documents MUST stay byte-identical.

#### Scenario: A single huge word

- **WHEN** a document contains one 8 MiB word and the default limits apply
- **THEN** the conversion succeeds within the documented allocation multiple of the
  rune count
- **AND** the PDF places the same glyphs as before the change

#### Scenario: A huge word under a small budget

- **WHEN** the same word converts with `MaxOperations` of 100000
- **THEN** the call returns `resource/limit` before the second glyph slice is built

#### Scenario: A run that needs mark offsets

- **WHEN** a run contains a combining mark with a non-zero offset
- **THEN** the painted segment carries that offset
- **AND** the copy is exact-sized and charged per glyph
