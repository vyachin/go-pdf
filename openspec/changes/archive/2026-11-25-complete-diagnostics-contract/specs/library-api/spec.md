# Spec Delta

## ADDED Requirements

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
