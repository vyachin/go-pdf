# Spec Delta

## ADDED Requirements

### Requirement: Assets can be read from a caller-supplied file system (RV5-48)

A conversion SHALL be able to read assets from a caller-supplied `fs.FS`. When
`Options.Source` is non-nil, every document-referenced image, stylesheet and font,
including `FallbackFont`, MUST be read through it; `BaseDir` and `FontDir` name
directories inside it and default to its root. Source paths MUST be relative and
MUST NOT contain `..`; nothing MUST be written. A missing asset MUST produce the
filesystem source's diagnostic, and identical bytes MUST produce a byte-identical
PDF.

#### Scenario: An embedded document with an image and a font

- **WHEN** a document from `embed.FS` references an image and declares a
  `@font-face` source, and the same bytes are converted from a directory
- **THEN** the source conversion succeeds and produces a byte-identical PDF

#### Scenario: A source path leaves the root

- **WHEN** `Source` is set and a reference is `../secret.png`, `/etc/hosts` or a
  `@font-face` source with `..`
- **THEN** the resource's load diagnostic is reported and no PDF is produced
- **AND** no host file is read

#### Scenario: A source asset is missing

- **WHEN** `Source` is set and an image, stylesheet or font it names does not exist
- **THEN** the diagnostics are `img/load`, `css/link-load` and `font/unavailable`
- **AND** they name the same reference a filesystem conversion would name

#### Scenario: The fallback font comes from the source

- **WHEN** `Source` is set and `FallbackFont` names a font file inside it
- **THEN** that file is read from the source and used for undeclared families

#### Scenario: A nil source keeps the filesystem

- **WHEN** `Options.Source` is nil
- **THEN** images, stylesheets and fonts are read from the host filesystem exactly
  as before
- **AND** the existing conversion tests stay green

#### Scenario: A source conversion writes nothing

- **WHEN** a document is converted with a source backed by `embed.FS` or
  `fstest.MapFS`
- **THEN** no file is created, modified or deleted
