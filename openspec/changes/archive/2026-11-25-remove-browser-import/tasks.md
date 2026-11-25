# Tasks

## 1. Remove the importer and update its API and CLI contract

- [x] 1.1 Delete `internal/browser`, remove `ImportBrowser` from both options structs, option forwarding, the CLI and WeasyPrint harness, simplify `prepare` to one profile-validation path, and remove the importer-only `Cascade.Declarations` helper and stale code comments. Verify production packages compile with `go build ./pdfgen ./cmd/pdfgen ./internal/engine ./internal/css ./internal/weasycompat` and no Go source imports `internal/browser` or exposes `ImportBrowser`.
- [x] 1.2 Replace importer acceptance tests with deterministic profile rejection coverage for both `Convert` and `Validate`, including ordinary and strict mode, no returned PDF and no `browser/*` diagnostics. Cover the former layout, value, text, cosmetic and media transformations using existing rejection tests where sufficient. Preserve the browser certificate bytes, update its notice, and retain native data-URI/background and profile-compatible certificate tests. Verify `go test ./pdfgen ./internal/css ./internal/engine` passes without updating supported rendering goldens.
- [x] 1.3 Replace the CLI browser-certificate acceptance test with rejection coverage that checks exit `1`, profile diagnostics, no new PDF and preservation of an existing output file. Verify `go test ./cmd/pdfgen` passes and still covers successful conversion of supported documents.
- [x] 1.4 Delete `docs/browser-import.md`, remove current import promises and links from `README.md`, `CONTRIBUTING.md`, `pdfgen/doc.go` and `docs/profile.md`, and add an Unreleased breaking-change entry to `CHANGELOG.md` with caller and template migration guidance. Verify `go doc ./pdfgen Options` shows no removed field and current consumer documentation links resolve; preserve historical changelog entries.

## 2. Reconcile the WeasyPrint corpus

- [x] 2.1 Remove import-policy wording from the harness, run all six cases through the normal profile, review the observed first error codes, and update `blocked_by` values in `testdata/weasyprint/samples.json` plus the blocker table in `docs/weasyprint-corpus.md`. Verify `go test ./internal/weasycompat -v` passes with deterministic diagnostics and unchanged corpus states, upstream file hashes and reference oracle.

## 3. Reconcile dependent planning artifacts

- [x] 3.1 Update `complete-diagnostics-contract/proposal.md`, `tasks.md` and `specs/library-api/spec.md` to remove the obsolete browser-import motivation, task 2.3 and invalid-page-size import scenario while preserving its general early option-validation work. Verify the change contains no requirement to retain `ImportBrowser` or browser lowering and `openspec validate complete-diagnostics-contract` passes.

## 4. Verify the complete removal

- [x] 4.1 Run `go test -shuffle=on ./...`, `go test -shuffle=on -race ./...` and the documented formatting and lint checks. Verify the full suite passes, supported text/rendering goldens are unchanged and no unrelated source or fixture changes are included.
- [x] 4.2 Review references across live code, current docs and active changes for obsolete importer promises, allowing removal/migration records and historical entries. Verify `openspec validate remove-browser-import --strict` passes and the remaining main-spec importer requirements are exactly those scheduled for removal by this change's delta.
