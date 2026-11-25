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
- **THEN** the call returns a `resource/limit` diagnostic before the conversion model
  or layout allocates

### Requirement: The work budget covers every conversion stage

`Convert` and `Validate` SHALL charge the shared work guard for every stage whose cost grows with the input: stylesheet handling, cascade and selector matching, font parsing, image decoding, text shaping, layout and PDF serialisation. Once the budget is exhausted the conversion MUST stop with a `resource/limit` diagnostic before the stage continues, and a cancelled context MUST be observed during those stages rather than after them.

#### Scenario: Image decoding is charged
- **WHEN** a document references images whose total pixels exceed the work budget while
  each image is under `MaxImagePixels`
- **THEN** the call returns a `resource/limit` diagnostic before decoding the images
  that would exceed the budget

#### Scenario: Font parsing is charged
- **WHEN** a font whose GPOS table is large enough to exhaust `MaxOperations` is loaded
  for a document
- **THEN** the call returns a `resource/limit` diagnostic instead of spending the full
  parse time

#### Scenario: Cancellation reaches a running stage
- **WHEN** the context is cancelled while a large image, font or page is processed
- **THEN** the call returns the context error, not a document diagnostic

#### Scenario: Within-budget work is unchanged
- **WHEN** a document converts inside the default limits
- **THEN** its PDF and diagnostics are byte-identical to the result before the change

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

### Requirement: Aggregate asset limits

`Limits` SHALL bound the totals of one conversion in addition to each file: total
encoded image bytes, total decoded image pixels and total font bytes. An asset that
would cross a total MUST fail with a `resource/limit` diagnostic before it is read or
decoded, and the defaults MUST let a normal business document convert without
configuration.

#### Scenario: Many assets cross the total

- **WHEN** a document references assets whose sum exceeds a configured total while
  each asset is under its per-file limit
- **THEN** the call returns a `resource/limit` diagnostic
- **AND** the assets past the limit are not read or decoded

#### Scenario: A normal document stays under the totals

- **WHEN** a document with one logo, one body font and one bold face converts with
  the default limits
- **THEN** the conversion succeeds and the assets are embedded once each

#### Scenario: The caller can raise a total

- **WHEN** the caller sets a larger total in `Limits`
- **THEN** the document converts and the PDF is byte-identical to the same document
  converted before the limit was reached

### Requirement: Default limits are readable by callers

The `pdfgen` package SHALL export the limits a zero `Limits` value resolves to, so a
caller that reads its own input can reject an oversized file before loading it. The
exported values MUST equal the values the conversion uses and MUST track changes to
the defaults.

#### Scenario: Defaults match the conversion

- **WHEN** a caller reads `pdfgen.DefaultLimits()`
- **THEN** every field equals the value the same conversion applies with `Options{}`

#### Scenario: The command uses the export

- **WHEN** the reference command checks an input's size
- **THEN** it uses the exported defaults rather than a hard-coded constant

### Requirement: Diagnostics identify the converted document

Every diagnostic about the document or one of its assets SHALL set `File` to
`Options.File`, or the documented placeholder when the caller set none, and SHALL set
line and column when a source position exists. Only `options/*` diagnostics MAY lack a
position. A missing image, a missing font and an exhausted resource limit MUST name
the document.

#### Scenario: A missing image

- **WHEN** a document named `invoice.html` references a missing image
- **THEN** the `img/load` diagnostic has `File == "invoice.html"` and a position

#### Scenario: A resource limit

- **WHEN** a document named `invoice.html` exhausts `MaxOperations`
- **THEN** the `resource/limit` diagnostic names `invoice.html`

#### Scenario: A rule in a linked stylesheet

- **WHEN** a linked stylesheet contains an unsupported declaration
- **THEN** the diagnostic names that stylesheet with its line and column

### Requirement: Options are validated before assets are read

`Convert` and `Validate` SHALL parse and validate `PageSize`, `Margin`, `Date` and
`Charset` before opening any asset, and SHALL report every invalid option in one
pass, independently of document errors.

#### Scenario: A bad option and a bad document

- **WHEN** `PageSize: "banana"` is combined with an unsupported CSS value
- **THEN** the report contains both `options/page-size` and the CSS diagnostic

#### Scenario: An out-of-range creation date

- **WHEN** `Options.Date` has year 10000
- **THEN** the call returns `options/date` and no PDF

### Requirement: Anchor warnings cover consumed targets only

`pagination/unknown-target` SHALL be reported only when a `target-counter` consumes a
missing anchor. An ordinary `<a href="#...">` MUST NOT warn, `href="#"` MUST be
ignored, and `Strict` MUST accept a document whose only links point nowhere.

#### Scenario: A placeholder link

- **WHEN** a document contains `<a href="#">top</a>` and no `target-counter`
- **THEN** conversion produces no `pagination/unknown-target`
- **AND** `Strict` mode succeeds

#### Scenario: A real missing target

- **WHEN** `a.toc::after { content: target-counter(attr(href), page) }` points at a
  missing id
- **THEN** the warning names that id and its source position

### Requirement: Error lists contain only errors

`Diagnostics.Err()` and `Error.Diagnostics` SHALL contain error-severity entries only.
Warnings SHALL remain available through `Result.Diagnostics` and `Validate`'s
diagnostic list.

#### Scenario: Errors and warnings together

- **WHEN** a document has an unsupported element (error) and an ignored script
  (warning)
- **THEN** `errors.As(err, &verr)` yields `verr.Diagnostics` with one entry
- **AND** `Result.Diagnostics` still contains both entries
