# reference-cli Specification

## Purpose

Defines the example command that ships with the library: what it accepts, what it
promises, and what it deliberately does not. Its purpose is to demonstrate one
supported way to call the package, not to expose every option of the engine.

## Requirements

### Requirement: One input, one output

The command SHALL convert exactly one HTML file into one PDF:
`pdfgen [-o output] [-charset name] input.html`. Without `-o` the output path MUST be the input
path with its extension replaced by `.pdf`. The command MUST NOT accept
subcommands.

#### Scenario: Default output path

- **WHEN** `pdfgen invoice.html` runs
- **THEN** `invoice.pdf` is written next to the input and nothing else is produced

#### Scenario: Explicit output path

- **WHEN** `pdfgen -o out.pdf invoice.html` runs
- **THEN** the PDF is written to `out.pdf`

#### Scenario: A removed subcommand is named as removed

- **WHEN** the first argument is a word such as `convert`, `validate` or `help`
- **THEN** the command prints that the subcommand was removed, shows the usage line, and exits with the usage code

### Requirement: Input encoding flag

`-charset name` SHALL name the input encoding explicitly and SHALL take precedence over the document's declaration and any byte order mark. The value MUST be one of the supported encodings; any other value MUST be a usage error: the command shows the usage line, exits `2`, and creates no output file.

#### Scenario: An explicit encoding

- **WHEN** `pdfgen -charset windows-1251 invoice.html` converts a document that declares nothing
- **THEN** the PDF is written and the Cyrillic text is correct

#### Scenario: An unknown encoding

- **WHEN** `pdfgen -charset shift_jis invoice.html` runs
- **THEN** the command shows the usage line, exits `2`, and no PDF is written

#### Scenario: Detection without the flag

- **WHEN** `pdfgen invoice.html` runs without `-charset` and the document declares windows-1251
- **THEN** the document converts without the flag

### Requirement: The command uses the public package only

The command SHALL be built from the `pdfgen` package and the standard library. It
MUST NOT import a package under `internal/`.

#### Scenario: The import boundary holds

- **WHEN** the command's direct imports are inspected
- **THEN** every one is `pdfgen` or a standard-library package

### Requirement: Asset roots follow the input file

The command SHALL resolve relative images, stylesheets and fonts against the
directory of the input file, so that a document and its assets can live in any
directory and still convert.

#### Scenario: Assets next to the document

- **WHEN** a document references `logo.png` that sits beside it
- **THEN** the image is embedded in the PDF

### Requirement: Diagnostics go to stderr and exit codes are stable

Warnings and errors SHALL be written to stderr, one per line, in the package's
diagnostic form. The command SHALL exit with `0` on success, `1` when the
document cannot be converted, and `2` on a usage error.

#### Scenario: An invalid document

- **WHEN** the input is outside the profile
- **THEN** each diagnostic is printed to stderr, stdout stays empty, and the exit code is `1`

#### Scenario: A missing input file

- **WHEN** the input path does not exist
- **THEN** the command prints one line to stderr and exits with `1`

#### Scenario: Bad invocation

- **WHEN** the command is called without an input path or with an unknown flag
- **THEN** it exits with `2`

### Requirement: The output is written only after a successful conversion

The command SHALL write the output file only after the package returned a
complete PDF. A failed conversion MUST leave an existing output file untouched
and MUST NOT create a partial one.

#### Scenario: Failure leaves the previous file

- **WHEN** the conversion fails and the output path already holds a PDF
- **THEN** that file is byte-identical to what it was before the run

#### Scenario: Failure creates no file

- **WHEN** the conversion fails and the output path does not exist
- **THEN** no file is created at that path

### Requirement: The command carries no compatibility promise

The command is an example. Its flags, its output format and its behaviour MAY
change in any release, including a minor one, and the documentation SHALL say so
where the command is described.

#### Scenario: A reader is told what is stable

- **WHEN** a reader looks for a compatibility promise for the command
- **THEN** the documentation states that the command is an example and that only the `pdfgen` package is versioned

### Requirement: The output never overwrites the input

The command SHALL refuse to write when the resolved output path and the input path
name the same file, including a case-insensitive filesystem, and MUST exit with a
usage or failure code instead of truncating the source. On success it SHALL write
through a temporary file in the destination directory and rename it into place, so a
failed write cannot leave a partial file at the output path.

#### Scenario: The input is named template.pdf

- **WHEN** `pdfgen template.pdf` runs and the default output equals the input
- **THEN** the command exits non-zero and the input is unchanged

#### Scenario: An explicit output equal to the input

- **WHEN** `pdfgen -o invoice.html invoice.html` runs
- **THEN** the command refuses and the source file is unchanged

#### Scenario: A failed write preserves the previous file

- **WHEN** the conversion succeeds but the rename or write fails
- **THEN** the destination still holds its previous bytes

### Requirement: The input is size-checked before reading

The command SHALL reject an input larger than the default `MaxInputBytes` before
loading it, using the limits the package exports, and SHALL report the same
`resource/limit` message the package uses.

#### Scenario: An oversized input

- **WHEN** the input file exceeds the default limit
- **THEN** the command exits with the failure code and reports the limit
- **AND** it does not read the whole file into memory
