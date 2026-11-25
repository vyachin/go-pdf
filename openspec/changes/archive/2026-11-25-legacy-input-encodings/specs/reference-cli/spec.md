# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

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
