# Spec Delta

## ADDED Requirements

### Requirement: Transparent PNG colour is unpremultiplied (RV5-33)

When a PNG carries partial alpha, the writer SHALL store unpremultiplied colour
samples plus a separate `/SMask` alpha. Palette and gray+alpha sources MUST be
handled like NRGBA: the source colour SHALL survive a viewer's alpha multiply,
and a semi-transparent pixel MUST NOT become darker than its source.

#### Scenario: Palette entry with partial transparency

- **WHEN** a PNG uses PLTE `(255,0,0)` with `tRNS` alpha `128`
- **THEN** the image XObject carries colour samples `ff 00 00`
- **AND** the `/SMask` carries `80`
- **AND** a viewer multiplying the two renders the source red

#### Scenario: Gray+alpha pixel with partial transparency

- **WHEN** a gray+alpha PNG stores gray `128` and alpha `128`
- **THEN** the colour stream carries `80`
- **AND** the mask carries `80`

#### Scenario: Opaque images are unchanged

- **WHEN** every pixel of a palette or NRGBA image is opaque
- **THEN** the colour samples are the source bytes
- **AND** no `/SMask` is attached

### Requirement: Font descriptor flags mark an italic face (RV5-34)

The `/FontDescriptor` `/Flags` value SHALL set bit 7 (64) when the embedded face
is italic or oblique and MUST leave it clear otherwise. The flag MUST agree with
the `/ItalicAngle` written in the same dictionary, so a consumer that reads
either one reaches the same conclusion.

#### Scenario: Italic face

- **WHEN** the embedded face reports a non-zero italic angle
- **THEN** `/Flags` is `68` (symbolic plus italic)
- **AND** `/ItalicAngle` is the face's angle

#### Scenario: Upright face

- **WHEN** the embedded face reports a zero italic angle
- **THEN** `/Flags` is `4`
- **AND** `/ItalicAngle` is `0`

### Requirement: Every emitted CID has a ToUnicode entry (RV5-35)

`ToUnicode` SHALL contain exactly one entry for every CID the writer emits. A
CID whose rune is U+0000 MUST be mapped to `<0000>` rather than skipped, so the
text layer covers the whole glyph run.

#### Scenario: A NUL rune is mapped

- **WHEN** a glyph run contains a glyph whose source rune is U+0000
- **THEN** the CMap carries a `bfchar` entry for that CID and `<0000>`
- **AND** the number of entries equals the number of emitted CIDs

#### Scenario: Ordinary text is unchanged

- **WHEN** a run contains only non-NUL runes
- **THEN** every CID keeps its existing mapping
- **AND** the extractable text is unchanged

### Requirement: Compressed streams are always closed (RV5-36)

The writer SHALL close the zlib writer on every path and MUST handle its error.
When compression fails or does not reduce the data, the object SHALL be written
uncompressed with a correct `/Length`; a failure MUST NOT produce a truncated
stream or leak the writer.

#### Scenario: Compression shrinks the data

- **WHEN** the compressed bytes are shorter than the input
- **THEN** the object carries `/Filter /FlateDecode`
- **AND** `/Length` equals the compressed length

#### Scenario: Compression fails or does not shrink

- **WHEN** the compressor returns an error or produces no saving
- **THEN** the object is written uncompressed
- **AND** `/Length` equals the original length
- **AND** the zlib writer is closed exactly once
