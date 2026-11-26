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

Document and asset diagnostics SHALL name their source file, or `Options.File` /
the placeholder when no more specific file is known. Attributable failures MUST
retain known line and column, including missing image/font references. Option
errors MAY omit file and position. Aggregate or output failures without a source
location MAY use zero coordinates but MUST name the document. Unknown locations
MUST NOT be fabricated.

#### Scenario: A missing image

- **WHEN** a document named `invoice.html` references a missing image
- **THEN** the `img/load` diagnostic has `File == "invoice.html"` and a position

#### Scenario: A resource limit

- **WHEN** a document named `invoice.html` exhausts `MaxOperations`
- **THEN** the `resource/limit` diagnostic names `invoice.html`

#### Scenario: A rule in a linked stylesheet

- **WHEN** a linked stylesheet contains an unsupported declaration
- **THEN** the diagnostic names that stylesheet with its line and column

#### Scenario: An aggregate output failure has no source location

- **WHEN** PDF serialization exhausts a conversion-wide budget without identifying one source node
- **THEN** the `resource/limit` diagnostic names the document
- **AND** line and column may be zero to mean unknown without fabricating an element path

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

### Requirement: A repeated measurement is built once per width (RV4-03)

Layout SHALL build a box's lines at most once per content width, whether the box is
measured for a break decision or laid out, and every line built for a measurement
MUST be charged to the work guard. A document MUST NOT be able to spend quadratic
time in repeated measurement of the same subtree.

#### Scenario: Nested avoid boxes stay linear

- **WHEN** a document nests 240 `break-inside: avoid` blocks with 300 words each
- **THEN** the conversion builds each block's lines once per width
- **AND** with a small `MaxOperations` the call returns `resource/limit` without
  spending quadratic time
- **AND** the same document under the default limits produces the same pages as a
  document with the same text but no nesting

### Requirement: Text shaping is charged per rune (RV4-04)

Text shaping SHALL charge the work guard once per rune it normalizes, maps and
measures, and the charge MUST cover the per-rune glyph and state storage. A
document at `MaxInputBytes` MUST fail with `resource/limit` before allocating memory
out of proportion to the budget.

#### Scenario: A single huge word is bounded

- **WHEN** a document contains a 2 MB word and `MaxOperations` is 100000
- **THEN** the call returns `resource/limit`
- **AND** the memory allocated before the stop is proportional to the budget, not
  to the input
- **AND** a document that fits the budget keeps its byte-identical output

### Requirement: PDF serialisation is charged by bytes (RV4-06)

PDF serialisation SHALL charge the guard in proportion to the bytes it writes, the
glyphs and CIDs it serialises, and the pixels it decodes, so `MaxOperations` bounds
writer cost and not only the number of operations on the page.

#### Scenario: A large output needs a large budget

- **WHEN** a document produces a PDF of several megabytes from a small input (for
  example 9000 distinct CIDs)
- **THEN** the recorded spend grows with the output size
- **AND** a small `MaxOperations` stops the conversion with `resource/limit` before
  the output is assembled

### Requirement: Validate reports everything Convert can decide before serialisation (RV4-09)

`Validate` SHALL run the same preparation and layout as `Convert`, including the
validation of document metadata. The package documentation MUST name PDF
serialisation, including its work budget, as the only stage `Validate` does not
cover. A document that `Validate` accepts MUST NOT fail `Convert` for a reason
decided before serialisation.

#### Scenario: Metadata is validated by both entry points

- **WHEN** a caller passes a title that is not valid UTF-8
- **THEN** `Convert` and `Validate` both report `options/title`
- **AND** `Validate` returns the same diagnostic list as `Convert`

#### Scenario: The uncovered stage is documented

- **WHEN** a document is accepted by `Validate` and fails in `Convert` only because
  serialisation exceeds the work budget
- **THEN** the error is a `resource/limit` from serialisation
- **AND** the documented scope of `Validate` names that stage

### Requirement: An invalid option still validates inline rules (RV4-28)

When an option is invalid, preparation MUST still validate the declarations the
document carries inline, including the rules of a `<style>` element, and report
them together with the option diagnostics in one pass. A linked asset MUST NOT be
opened for a request that cannot proceed.

#### Scenario: A style element and a bad page size

- **WHEN** `PageSize: "banana"` is combined with `<style>p{display:flex}</style>`
- **THEN** the report contains `options/page-size` and `css/unsupported-value`
- **AND** no linked stylesheet or font is opened

### Requirement: A missing font names its declaration (RV4-37)

A `font/unavailable` diagnostic SHALL carry the file name of the document and the
position of the `src` declaration that named the missing file, so the report points
at the line the author can fix.

#### Scenario: A missing font file

- **WHEN** a document declares `@font-face { src: url(missing.ttf) }`
- **THEN** the diagnostic is `font/unavailable`
- **AND** it carries the document name and the line and column of the `src`
  declaration

### Requirement: The charset check accepts the empty name (RV4-59)

`CharsetSupported` SHALL report true for the empty string, because the empty
`Options.Charset` is a valid request that means "detect". The documentation MUST
state that a caller checking only named encodings writes
`name != "" && CharsetSupported(name)`.

#### Scenario: The empty name is valid

- **WHEN** a caller checks `CharsetSupported("")`
- **THEN** the result is true
- **AND** `CharsetSupported("shift_jis")` stays false

### Requirement: A budget failure stops dependent diagnostics (RV4-62)

Once the work guard reports exhaustion or cancellation, later stages MUST NOT add
diagnostics that depend on the work that was skipped, such as a missing font
reported for a face the exhausted budget never tried to resolve. The report MUST
contain the budget or context cause and no misleading follow-on error.

#### Scenario: An exhausted font budget does not report a missing family

- **WHEN** a document's font work exhausts `MaxOperations` before the fallback
  family is resolved
- **THEN** the report contains `resource/limit`
- **AND** it does not also contain `text/font` for that family

### Requirement: One pipeline serves both entry points (IMP-02)

`Convert` and `Validate` SHALL share one implementation of preparation, layout and
metadata checks, with only the output stage differing. A behaviour added to a shared
stage MUST reach both entry points without a second code path or a duplicated test.

#### Scenario: A new preparation check (IMP-02)

- **WHEN** a check is added to preparation
- **THEN** both entry points report it
- **AND** the suite does not need two fixtures for the same check

### Requirement: Every budgeted stage states a cost and has an invariant test (IMP-03)

For every stage the work guard covers, the code SHALL document what one unit means,
and the suite MUST contain an invariant test: for an adversarial fixture at
`MaxOperations = N`, time and allocations stay within a constant multiple of `N`. A
stage that cannot state its cost MUST NOT be described as covered by the budget.

#### Scenario: A new expensive stage (IMP-03)

- **WHEN** a stage that grows with the input is added
- **THEN** it charges the guard and the invariant test covers it
- **AND** a stage that allocates per unit without a charge fails that test

### Requirement: Asset policy is consistent within a conversion

Permitted references to one image or font file SHALL share one successful asset
record and count bytes once per conversion. Identity MUST distinguish kind and
decoding inputs; every reuse MUST enforce the reference's root and access policy.
Stylesheet inclusions MUST retain cascade order. State MUST be conversion-local,
released on every exit, and preserve typed failures with their source and cause.

#### Scenario: One font has two permitted roles

- **WHEN** a permitted canonical font file is used by `@font-face` and explicit `FallbackFont` in one conversion
- **THEN** the bytes are read, parsed and admitted once while both roles resolve correctly

#### Scenario: A cached file is outside another root

- **WHEN** one reference is permitted to load a file and a later template reference would escape its own configured root
- **THEN** the later reference is rejected even if the file is already cached

#### Scenario: A stylesheet is included twice

- **WHEN** a local stylesheet is linked twice with another sheet between its inclusions
- **THEN** both inclusions participate at their original cascade positions even if file reading and immutable parsing are reused

#### Scenario: Independent conversions use different assets

- **WHEN** two concurrent conversions use equal relative names under different roots
- **THEN** their records, totals, failures and output remain isolated

#### Scenario: A resource load fails

- **WHEN** an asset cannot be loaded or exceeds a limit
- **THEN** the typed failure retains its resource kind, original reference and cause and unsuccessful reads are not committed as successful assets

### Requirement: Layout diagnostics preserve available source provenance

Layout SHALL retain available source provenance through splitting, shaping and
line construction. Attributable font, glyph and overflow failures MUST identify
the original file, nonzero source line/column and owning element. Generated text
MUST use its generating declaration when known. Positions MUST use the existing
original-byte mapper; normalization MUST NOT cause fabricated precision.

#### Scenario: A glyph is missing on a later line

- **WHEN** a multiline document contains a character the selected font cannot render inside a nested span
- **THEN** both entry points report `text/glyph` at the originating text or owning span with a nonzero source position and element path

#### Scenario: A margin-box glyph is missing

- **WHEN** a margin-box `content` declaration generates an unavailable character
- **THEN** the diagnostic identifies the stylesheet declaration that generated it when that source is available

#### Scenario: Decoded text retains its original source

- **WHEN** a supported legacy-encoded document fails during text layout after Cyrillic text
- **THEN** the diagnostic position refers to the original file through its existing byte mapper

#### Scenario: Normalization changes character boundaries

- **WHEN** entities or canonical composition prevent an exact glyph-to-byte mapping
- **THEN** the diagnostic uses the known owning source location and does not invent an exact character offset

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
