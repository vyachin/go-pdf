# reference-cli Specification

## Purpose

Defines the example command that ships with the library: what it accepts, what it
promises, and what it deliberately does not. Its purpose is to demonstrate one
supported way to call the package, not to expose every option of the engine.

## Requirements

### Requirement: One input, one output

The command SHALL convert exactly one HTML file into one PDF:
`pdfgen [-o output] input.html`. Without `-o` the output path MUST be the input
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

### Requirement: The command uses the public package only

The command SHALL be built from the `pdfgen` package and the standard library. It
MUST NOT import a package under `internal/`, and a check in continuous
integration MUST fail when its direct imports contain one.

#### Scenario: The import boundary holds

- **WHEN** the import check inspects the command's direct imports
- **THEN** no path under `internal/` is present

#### Scenario: A broken boundary fails the build

- **WHEN** a change makes the command import an internal package
- **THEN** continuous integration fails before the change merges

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
