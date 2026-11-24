# Tasks

## 1. Page orientation

- [x] 1.1 Parse the `size` grammar: size name, one or two lengths, `portrait`/`landscape` separately and together with a name; an unknown word and orientation together with lengths — errors — check: the case table in `internal/css` passes
- [x] 1.2 Add `css.PageSizeSpec` and `layout.ResolvePageSize(spec, fallback)`, apply in `ParsePageStyle` — check: `A4 landscape` yields 297x210, `landscape` with fallback A5 yields 210x148
- [x] 1.3 Update `@page size` validation: `A4 sideways` and `210mm 297mm landscape` are rejected, `A4 landscape` is accepted — check: `internal/profile` tests
- [x] 1.4 End-to-end test: a landscape PDF has width 297 mm and height 210 mm, headers and footers are placed on the new sides — check: `internal/engine` test reads `/MediaBox` and the header text
- [x] 1.5 Update `docs/profile.md` with a section on `size` and orientation — check: the example from the document runs via `pdfgen validate`

## 2. Rule source in diagnostics

- [x] 2.1 Give `css.Stylesheet` a source (`File`, `Src`, line index) and a position method; `Rule` and `Declaration` SHALL reference their sheet — check: unit test of the position by offset for a sheet from a file
- [x] 2.2 Switch `profile` to positions via the sheet; inline `<style>` keeps pointing to HTML lines — check: the existing position and diagnostics format tests are green
- [x] 2.3 Test: a declaration from an included file yields `print.css:12:4`, not an HTML line — check: `internal/profile` test on a synthetic sheet

## 3. External stylesheets

- [x] 3.1 Replace `ExtractStyleSheets` with `CollectStylesheets(doc, opts)`, which walks the DOM in order of appearance and collects `<style>` and `<link rel="stylesheet">` — check: the order test returns sheets in document order
- [x] 3.2 Load a local file relative to `BaseDir`, support an absolute path; a missing file — `css/link-load`, `http(s)` — `css/link-remote`, `data:` — `css/link-unsupported` — check: four scenarios in the tests
- [x] 3.3 Confine paths: with `BaseDir` set, the resolved path SHALL remain inside it — check: the test on `../../outside.css` gives an error, the file is not read
- [x] 3.4 Implement the `rel` and `media` rules: `stylesheet` is included, other `rel` values are ignored, `print`/`all`/empty are applied, `screen` is skipped — check: three tests without diagnostics
- [x] 3.5 Pass the slice of sheets to `css.NewCascade` variadically and verify the cascade order — check: `<style>`, `<link>`, `<style>` — the last declaration wins
- [x] 3.6 Update `docs/profile.md`: `<link>` is supported, the network is forbidden, `BaseDir` is mandatory for path confinement — check: the section exists and matches the CLI behavior

## 4. Scripts and strict mode

- [x] 4.1 Ignore `<script>`: warning `html/ignored-script`, `script { display: none }` in the engine's stylesheet, the content does not get into the PDF — check: the test verifies the warning and the absence of the script text in the output
- [x] 4.2 Add `engine.Options.Strict` and `profile.Options{Strict}`, turning warnings into errors — check: the same template with a script under `Strict` gives an error and does not create a PDF
- [x] 4.3 Add the `-strict` flag to the `validate` and `convert` commands, update the help — check: CLI test on exit code 1 and the error text
- [x] 4.4 Update `README.md`: scripts are not executed, `-strict` for CI — check: the CLI section contains the flag, the example runs

## 5. Fixture and acceptance

- [x] 5.1 Write `testdata/bigtable.html`: 35 rows by 11 columns, Russian text, `thead`, `tfoot`, `@page { size: A4 landscape }` — check: `pdfgen validate` without errors, the build produces at least three pages
- [x] 5.2 Verify the pagination of the fixture: the header repeats on every page, `tfoot` totals only on the last one — check: the test parses the page text
- [x] 5.3 Add the fixture's golden text to the engine corpus and verify deterministic output — check: `go test ./internal/engine -update`, then a repeat run without `-update`
- [x] 5.4 Run acceptance: `go vet ./...`, `go test ./...`, `go test ./internal/mpdfcompat`, `openspec validate page-orientation-and-external-css` — check: everything is green, warnings only about the spec language

## Workflow follow-up

- Archive the change after review and verification against the fixture.
- Sync the `css-profile` and `pagination` deltas into the main specs when archiving.
