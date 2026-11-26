# Spec Delta

## ADDED Requirements

### Requirement: Concurrent reads of one asset are counted once (RV5-37)

When several callers of one conversion request the same asset identity at the same
time, the asset SHALL be read and admitted once. Every caller MUST receive an
equivalent record, the bytes MUST be charged to the aggregate total once, and a
failure MUST reach every caller with the same typed cause. A peer MUST NOT be
rejected because another peer's reservation is still pending.

#### Scenario: Eight concurrent readers of one file

- **WHEN** eight goroutines of one conversion read the same 1 MiB file while the
  aggregate total is 1.5 MiB
- **THEN** all eight receive a usable record
- **AND** the file is read once and the total records 1 MiB, not 8 MiB

#### Scenario: A concurrent read fails

- **WHEN** several callers read one identity whose bytes cannot be admitted
- **THEN** every caller receives the same failure kind and cause
- **AND** nothing is committed to the total or to the cache

### Requirement: Aggregate totals charge the bytes actually admitted (RV5-38)

The aggregate byte total SHALL be charged for the bytes actually admitted, not for
a size reported before the read. A file whose reported size differs from the bytes
read MUST be charged its actual length or refused before admission; returning bytes
while charging nothing MUST NOT happen. A refund MUST return exactly the amount
charged, so totals cannot drift or become negative.

#### Scenario: A file that reports zero and yields bytes

- **WHEN** an asset reports size 0 before the read and yields 1 MiB
- **THEN** the conversion either refuses it before admission or charges 1 MiB
- **AND** it never returns the bytes while the total stays unchanged

#### Scenario: A file grows between the size check and the read

- **WHEN** an asset is larger when read than the size that was checked
- **THEN** the difference is charged or the asset is refused
- **AND** a later successful asset sees the corrected remaining total

#### Scenario: A failed decode refunds exactly

- **WHEN** an admitted asset cannot be decoded and is discarded
- **THEN** the total returns to the value it had before that admission
- **AND** a repeated failure cannot drive the total below zero

### Requirement: Open failures are classified by containment (RV5-39)

An asset open failure SHALL be classified from the containment check, not from the
text of the error. A failure inside a configured root — permission denied, an
unreadable file — MUST be reported as a load failure (`css/link-load`, `img/load`,
`font/unavailable`); only a reference that actually leaves the root MUST be
reported as a path escape (`css/link-path`).

#### Scenario: A permission failure on a file named escapes.css

- **WHEN** a stylesheet inside the configured root is named `escapes.css` and its
  mode denies reading
- **THEN** the conversion reports `css/link-load`
- **AND** it does not report a path escape

#### Scenario: A real path escape

- **WHEN** a stylesheet reference is `../outside.css`
- **THEN** the conversion reports `css/link-path`

#### Scenario: A symlink that leaves the root

- **WHEN** a stylesheet reference is a symlink whose target is outside the root
- **THEN** the conversion reports `css/link-path`

### Requirement: Metadata is validated before assets are opened (RV5-40)

The document metadata that produces `options/title`, `options/author`,
`options/subject` and `options/keywords` SHALL be validated with the other options,
before any linked asset or font is opened. When it is invalid, the conversion MUST
report the option diagnostic, MUST validate inline styles, and MUST NOT open a
linked asset, so a missing or broken asset cannot hide the option error.

#### Scenario: An invalid title and a missing stylesheet

- **WHEN** `Options{Title: "bad \xff"}` is combined with a link to a missing
  stylesheet
- **THEN** the report contains `options/title`
- **AND** the stylesheet is not opened, so no asset failure hides it

#### Scenario: An invalid title and a valid stylesheet

- **WHEN** the same title is combined with a stylesheet that exists
- **THEN** the report contains `options/title` and no PDF is produced

#### Scenario: Valid metadata and a missing stylesheet

- **WHEN** the metadata is valid and a linked stylesheet is missing
- **THEN** the report contains `css/link-load` and no option diagnostic
