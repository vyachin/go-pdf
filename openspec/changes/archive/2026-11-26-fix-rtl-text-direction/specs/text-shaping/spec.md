# Spec Delta

## ADDED Requirements

### Requirement: Right-to-left runs are laid out in visual order (RV5-04)

The engine SHALL reorder the runs of every line into visual order per UAX#9, using
`golang.org/x/text/unicode/bidi`, with the base direction taken from the nearest
`dir` attribute (`ltr`, `rtl` or `auto`) or, when there is none, from the first
strong character. Shaping SHALL stay in logical order and the text layer MUST keep
logical order. A document without right-to-left characters MUST produce the same
bytes as before this change.

#### Scenario: A Hebrew sentence

- **WHEN** the document contains `<p>שלום עולם</p>` and the font covers Hebrew
- **THEN** the first logical word is painted on the right and the visual sequence
  is `עולם שלום`
- **AND** extracted text is the logical sequence `שלום עולם`

#### Scenario: Mixed directions

- **WHEN** a paragraph contains `abc שלום 123`
- **THEN** the Latin runs keep left-to-right order, the Hebrew run is reversed, and
  the digits keep their left-to-right order
- **AND** no character is duplicated, dropped or moved to another line

#### Scenario: An explicit base direction

- **WHEN** a paragraph carries `dir="rtl"` and starts with a Latin letter
- **THEN** the paragraph base direction is right-to-left because `dir` wins over
  first-strong detection
- **AND** `dir="auto"` instead selects the first strong character

#### Scenario: Left-to-right text is unchanged

- **WHEN** a document contains no right-to-left characters
- **THEN** the produced PDF is byte-identical to the output before this change

### Requirement: Unsupported joining scripts are refused with a diagnostic (RV5-04)

A run whose script needs contextual shaping that the engine does not implement
(Arabic, Syriac, N'Ko and the Indic scripts) MUST NOT be painted unjoined or in
logical order. The conversion SHALL fail with the error diagnostic
`text/unsupported-script` naming the script and the source position, and
`Validate` MUST report the same diagnostic. Hebrew, which needs reordering but no
joining, SHALL render.

#### Scenario: An Arabic paragraph

- **WHEN** a document contains `<p>مرحبا بالعالم</p>`
- **THEN** `Convert` returns `text/unsupported-script` at the paragraph position
- **AND** no PDF is produced
- **AND** `Validate` returns the same diagnostic without writing a PDF

#### Scenario: Hebrew is supported

- **WHEN** a document contains only Hebrew and Latin text
- **THEN** it converts with no diagnostic
- **AND** the Hebrew runs are in visual order

#### Scenario: Mixed Arabic and Latin

- **WHEN** one paragraph mixes Arabic and Latin words
- **THEN** the whole document is refused with the same diagnostic
- **AND** no partially shaped PDF is returned
