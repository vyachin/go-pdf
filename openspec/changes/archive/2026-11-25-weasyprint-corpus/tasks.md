# Tasks

## 1. Corpus data and provenance

- [x] 1.1 Copy the five upstream directories (`invoice`, `letter`, `poster`, `ticket`, `report`) from `CourtBouillon/weasyprint-samples` at commit `6dc675b05a5cba59576dfaa8881ea6ccdccb13cc` into `testdata/weasyprint/` byte for byte, including the reference PDFs; verify with `diff -r` against a fresh tarball of that commit that no file was edited, added or dropped.
- [x] 1.2 Add the licence text for every bundled font family (six OFL texts plus Apache-2.0 for Coming Soon, which the fonts' name tables show is not under the OFL) under `testdata/weasyprint/licenses/` with an `index.json` mapping every font file to its licence, and write `testdata/weasyprint/NOTICE.md` with the upstream URL, the pinned commit, the upstream licence statement, the reference-PDF role, and the exclusion of `book` with its reason; verified every font file in the corpus is covered by exactly one licence entry and that `NOTICE.md` names the exclusion.
- [x] 1.3 Write `testdata/weasyprint/samples.json`: the source pin, a SHA-256 for every vendored file, and the six cases (`invoice`, `letter`, `poster`, `flyer`, `ticket`, `report`) with their directory, HTML file, optional stylesheet and `blocked` state; verify a manifest lint in the harness reports no case without a directory, no directory without a case and no unlisted file.

## 2. Honest page counting in pdfdebug

- [x] 2.1 Add `PageCount` and `PageTexts` to `internal/pdfdebug`, walked from the page tree (`/Kids`) and each page's `/Contents` reference, returning one entry per page including pages with no text; verify unit tests cover page order, an image-only page and an empty page, using PDFs produced by the engine.
- [x] 2.2 Prove the existing readers are untouched: `Pages` and `Text` keep their behaviour and the engine text goldens, the mPDF goldens and `go test ./internal/mpdfcompat ./internal/engine` pass without any golden regeneration.

## 3. The weasycompat harness

- [x] 3.1 Implement text normalisation and the token multiset (NFKC, NBSP folding, soft-hyphen and zero-width removal, maximal letter/digit runs, case preserved, duplicates counted) with table-driven unit tests for each rule.
- [x] 3.2 Implement manifest loading and provenance verification: every vendored file matches its hash, an unknown file or a missing file fails, and the upstream commit is reported; verify with a temporary copy of a sample directory that is mutated in each of the three ways.
- [x] 3.3 Implement the `blocked` assertions — conversion returns an error, at least one error diagnostic exists, the first error code equals `blocked_by`, two runs produce identical diagnostics, a panic is reported with its stack — and verify each assertion with a synthetic case and with the state-mismatch failure ("blocked case converted").
- [x] 3.4 Implement the `measure`/`accepted` golden assertions: page text with page separators compared byte for byte, `-update` rewriting only the processed cases' goldens, a missing golden failing with the regeneration command, and a golden without a case failing; verify by round-tripping a fixture through `-update` and by mutating one character of a golden.
- [x] 3.5 Implement the oracle: load and verify the reference PDF SHA-256, compare page count and document-wide token multiset, report missing and extra tokens for `accepted`, and log both numbers for `measure`; verify with unit tests on synthetic oracle data, including a page-count mismatch and a one-token loss.
- [x] 3.6 Implement `-update-oracle`, which runs `pdftotext` once per reference PDF, records the extractor identity and the reference hash, and refuses to write when the reference hash does not match the manifest; generate the six oracle files locally (`brew install poppler`) and verify the committed files are byte-identical on a second run.
- [x] 3.7 Wire the six real cases with `ImportBrowser: true`, in-memory `<link>` injection for `poster` and `flyer`, sample-relative `BaseDir`/`FontDir` and no fallback font; capture the first blocking error code of each case into `samples.json`; verify `go test ./internal/weasycompat -v` is green and logs six cases, their states and their blockers.

## 4. Documentation

- [x] 4.1 Write `docs/weasyprint-corpus.md`: what the corpus is and is not, the three states and what each asserts, the commands (`go test`, `-update`, `-update-oracle`), the poppler version the committed oracle was produced with, the upstream refresh procedure, the licence and exclusion notes, the size cost, and the current gap table taken from a verbose run; verify every documented command exists as a flag or test name in the code.
- [x] 4.2 Link the corpus from `CONTRIBUTING.md` (tests section and update-command list) and keep the README free of contributor material; verify the links resolve and that the mPDF paragraph and the golden paragraphs still read correctly.
- [x] 4.3 Run `openspec validate weasyprint-corpus --strict` and fix what it reports about this change's specs and artifacts.

## 5. Integration

- [x] 5.1 Run `go vet ./...`, `go test ./...`, `go test -race ./...` and the CI golden job locally with `PDF_REQUIRE_VALIDATOR=1`; verify `git status` shows no change under `testdata/golden/` or `testdata/mpdf/`, and that the only new testdata is `testdata/weasyprint/`.
- [x] 5.2 Confirm the corpus runs offline: run `go test ./internal/weasycompat` with `pdftotext` removed from `PATH`, and confirm that only `-update-oracle` needs it; measure the final corpus size and record it in the docs if it differs from ≈7.6 MB.
