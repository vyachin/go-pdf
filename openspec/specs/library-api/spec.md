# library-api Specification

## Purpose

Defines the contract an embedding Go program relies on: which import surface is
supported, what a conversion is allowed to touch, how the package behaves under
concurrency and hostile input, and which identifiers stay stable across
releases.

## Requirements

### Requirement: The public package is the supported import surface

A program that embeds the engine SHALL need nothing but the `pdfgen` package. No
supported use case may require importing a package under `internal/`, and the
package SHALL work as a dependency of another module without changes.

#### Scenario: An external module compiles against the package

- **WHEN** a separate Go module imports `github.com/vyachin/go-pdf/pdfgen` and calls `Convert`
- **THEN** the module builds and the conversion produces a PDF

#### Scenario: The package is tested from outside

- **WHEN** the package's own tests are compiled
- **THEN** the external test package imports only `pdfgen` and exercises the same calls a consumer makes

### Requirement: A conversion writes nothing to disk

`Convert` and `Validate` SHALL NOT create, modify or delete any file. They MAY
read only the assets the document references, inside the configured asset roots.
Writing the returned PDF MUST remain the caller's decision.

#### Scenario: The working directory is unchanged

- **WHEN** a document is converted
- **THEN** no file appears, changes or disappears in the working directory

#### Scenario: A missing asset does not create output

- **WHEN** a document references an image that does not exist
- **THEN** the call returns a diagnostic instead of a PDF, and no file is written

### Requirement: Arbitrary input never panics

`Convert` and `Validate` SHALL return either a result or an error for any byte
sequence. They MUST NOT panic, including on truncated HTML, malformed CSS,
invalid images, malformed fonts and deeply nested markup.

#### Scenario: Fuzzed input stays contained

- **WHEN** the fuzz target feeds arbitrary bytes to the public entry points
- **THEN** every iteration ends in a result or an error, and the process does not crash

#### Scenario: An over-limit document is rejected

- **WHEN** a document exceeds a configured limit, such as nesting depth or page count
- **THEN** the call returns a `resource/limit` diagnostic before the expensive allocation

### Requirement: Independent conversions are safe to run concurrently

The package SHALL keep no mutable global state. Concurrent calls to `Convert` and
`Validate` MUST be safe, and their output MUST be identical to the output of the
same calls made sequentially.

#### Scenario: Parallel conversions agree

- **WHEN** the same template is converted by several goroutines at once under the race detector
- **THEN** every call returns the same bytes and the race detector reports nothing

#### Scenario: Different documents do not interfere

- **WHEN** different templates are converted concurrently
- **THEN** each result equals the result of converting that template alone

### Requirement: Diagnostic codes are stable identifiers

Every diagnostic SHALL carry a `Code`. Within a major version the set of codes
MAY grow, but an existing code MUST NOT change its spelling or its meaning, and
a document that produced a code MUST keep producing it.

#### Scenario: An unsupported property is reported by code

- **WHEN** a template uses a property outside the profile
- **THEN** the diagnostic carries the code the documentation names for that failure

#### Scenario: The same document reports the same codes

- **WHEN** the same invalid document is converted twice
- **THEN** both runs report the same codes in the same order

### Requirement: The zero options value is a valid request

Passing a zero `Options` SHALL be a complete request: the documented default page
size, margin, creation date and limits MUST apply, and a document inside the
profile MUST convert without any option being set.

#### Scenario: Defaults need no configuration

- **WHEN** a profile document that declares its font is converted with `Options{}`
- **THEN** the result uses A4, a 20 mm margin and the fixed creation date
