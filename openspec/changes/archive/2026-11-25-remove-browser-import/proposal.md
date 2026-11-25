# Proposal

## Why

Browser import adds a second acceptance path that rewrites unsupported HTML and
CSS, replaces diagnostics and validates the result again. Removing this optional
compatibility layer simplifies the engine and its public options around one
closed profile.

## What Changes

- **BREAKING:** remove `ImportBrowser` from both `pdfgen.Options` and
  `engine.Options`, including option forwarding and the CLI and corpus callers.
- Delete `internal/browser`, the import branch in `prepare`, its repeated
  validation and the raw `Cascade.Declarations` helper used only by the importer.
- **BREAKING:** reject templates that relied on browser lowering with ordinary
  profile diagnostics. The reference CLI no longer accepts the browser certificate
  and no conversion emits `browser/*` diagnostics.
- Replace import acceptance tests with profile rejection coverage, and update the
  WeasyPrint corpus's committed blockers from observed profile diagnostics.
- Remove current documentation that offers browser import and document the API
  and behavior changes in the next release notes.
- Track reconciliation of the pending `complete-diagnostics-contract` change's
  obsolete browser-import task and scenario as implementation work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `browser-import`: remove all seven requirements that define the optional
  lowering layer, its diagnostics and its certificate acceptance promise. The
  existing `css-profile` contract becomes the sole acceptance path.

## Impact

- **Engine and API:** `internal/engine/{engine,convert}.go`, `pdfgen/pdfgen.go`,
  `internal/browser/`, `internal/css/cascade.go`, `cmd/pdfgen/main.go` and
  `internal/weasycompat/harness.go`.
- **Tests and fixtures:** importer tests, engine and CLI certificate tests, the
  certificate fixture notice, and `testdata/weasyprint/samples.json`. Preserve the
  browser certificate as a rejection fixture and the separate profile-compatible
  visual certificate as a supported rendering fixture.
- **Documentation:** `README.md`, `CONTRIBUTING.md`, `pdfgen/doc.go`,
  `docs/profile.md`, `docs/weasyprint-corpus.md`, `docs/browser-import.md` and
  `CHANGELOG.md`.
- **Compatibility:** removing a public option and retiring diagnostic behavior is
  a breaking library change; release handling follows the existing versioning
  policy. The reference command also changes its accepted input set.
- **Scope:** native background images, embedded PNG/JPEG data URIs, fonts,
  supported selectors, deterministic output and `Strict` warning promotion remain
  part of the profile. No dependencies, replacement importer or new layout
  support are introduced.
