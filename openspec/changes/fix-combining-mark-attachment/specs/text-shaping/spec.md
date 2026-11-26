# Spec Delta

## ADDED Requirements

### Requirement: A mark after a base attaches to that base (RV5-10)

When a non-mark glyph is stored, the attachment state MUST be reset so the next
combining mark is positioned from that base using `MarkBase` or `MarkLig`. A mark
MUST NOT reuse an anchor belonging to an earlier cluster, and a mark that has no
base in its run MUST remain the existing missing-mark diagnostic.

#### Scenario: The second acute in о́о́

- **WHEN** the text is `о` U+0301 `о` U+0301
- **THEN** the first acute is painted over the first `о`
- **AND** the second acute is painted over the second `о`, not over the first

#### Scenario: The second acute in а́б́

- **WHEN** the text is `а` U+0301 `б` U+0301
- **THEN** each acute uses the anchor of the base that immediately precedes it

#### Scenario: A mark at the start of a run

- **WHEN** a combining mark starts the shaped run and no base precedes it
- **THEN** the conversion reports the existing missing-mark diagnostic
- **AND** the mark is not painted at the pen position

### Requirement: Mark-to-mark attachment only follows an adjacent mark (RV5-10)

`MarkMark` SHALL be attempted only when the immediately preceding glyph is a mark
of the same cluster. When that anchor is absent, the shaper MUST fall back to the
current base's `MarkBase` anchor rather than to an older mark. A mark after a new
base MUST NOT use `MarkMark` at all.

#### Scenario: Two marks on one base

- **WHEN** the text is `о` U+0301 U+0302, two marks on one base, and the font
  provides a `MarkMark` anchor for the second
- **THEN** the second mark is attached mark-to-mark to the first
- **AND** both marks are painted above the same `о`

#### Scenario: A new base clears the mark state

- **WHEN** the sequence is base, mark, base, mark
- **THEN** the fourth glyph is attached with `MarkBase` or `MarkLig` to the second
  base
- **AND** no `MarkMark` lookup from the first cluster is used

#### Scenario: A font without a mark-to-mark anchor

- **WHEN** two consecutive marks follow one base and the font has no `MarkMark`
  anchor for the second
- **THEN** the shaper falls back to the current base's `MarkBase` anchor
- **AND** the document either renders with that anchor or reports the missing-mark
  diagnostic, never a stale position

### Requirement: Mark attachment stays over its base (RV5-10)

An attached mark's painted horizontal position SHALL lie over the glyph cell of
the base it attaches to, within the shaped run's extent. When the font's anchor
would place the mark over a different base or outside the run, the conversion
MUST report the existing missing-mark diagnostic instead of painting it there.

#### Scenario: An anchor outside the base

- **WHEN** a crafted font declares a mark anchor that places the mark before the
  run start or beyond the run end
- **THEN** the conversion reports the missing-mark diagnostic
- **AND** the glyph is not painted at the invalid position

#### Scenario: A normal anchor is unaffected

- **WHEN** the font's anchor places the mark inside the base's advance box
- **THEN** the mark is painted at that position with no diagnostic
- **AND** the run's advances are unchanged
