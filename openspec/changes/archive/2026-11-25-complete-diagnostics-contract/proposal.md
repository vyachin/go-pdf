# Proposal

## Why

The diagnostic contract and its implementation disagree. `Diagnostic.File` is
documented as the name passed in `Options.File`, but a missing image, a missing font
and an exhausted limit all arrive with `File=""` and `Line=0`, so a CI consumer cannot
attribute them. `Options.PageSize` and `Margin` are validated only after the document
and its assets, and are masked by the first document error. `Options.Date`
outside years 0..9999 produces a
`/CreationDate` that `pdfcpu` strict rejects, with no diagnostic. Every ordinary
`<a href="#...">` produces `pagination/unknown-target` (even `href="#"`, even with no
`target-counter`), and `Strict` then rejects a valid document. `Diagnostics.Err()`
and `Error.Diagnostics` include warnings although both say they hold errors. The
profile never validates attributes, so `<div hidden>` prints its content.

## What Changes

- Every diagnostic about the document SHALL set `File` and, where a position exists,
  line and column; only `options/*` may lack them.
- All options SHALL be parsed and validated before assets are read, and every invalid
  option reported in one pass regardless of document errors.
- `Date` outside years 0..9999 SHALL be `options/date`.
- `pagination/unknown-target` SHALL fire only for anchors a `target-counter` uses;
  `href="#"` is ignored; `Strict` accepts such documents.
- `Error.Diagnostics` SHALL contain error-severity entries only.
- Unknown attributes SHALL be reported and `hidden` SHALL hide its subtree.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: diagnostics identify the file, options fail early, anchor warnings
  are targeted, and error lists contain only errors.
- `css-profile`: attributes are validated and `hidden` hides content.

## Impact

- **Code**: `pdfgen/pdfgen.go`, `internal/engine/convert.go`, `internal/layout`,
  `internal/profile/profile.go`, `internal/diag`.
- **Output**: more diagnostics carry positions; a bad option is reported alongside
  document errors; `hidden` content disappears; `href="#"` no longer fails `Strict`.
- **Tests**: one regression per scenario; the CLI output format stays unchanged.
