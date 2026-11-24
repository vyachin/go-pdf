# Tasks

## 1. Font object graph (PDF-01)

- [x] 1.1 Allocate five objects per font (Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode) and link them exactly once — verification: unit test resolves `/DescendantFonts` and finds `/Subtype /CIDFontType2`
- [x] 1.2 Follow the whole chain in a test: Type0 → CIDFontType2 → FontDescriptor → FontFile2, plus the separate ToUnicode reference — verification: the test fails on the old four-id layout
- [x] 1.3 Verify Cyrillic text still extracts and the document passes an independent validator — verification: engine test with text extraction and `pdfcpu validate`

## 2. Malformed font tables (SEC-01)

- [x] 2.1 Validate the header length of a class definition per format before reading any field — verification: unit test with a four-byte format 1 header returns safely
- [x] 2.2 Apply the same per-format bounds checks to coverage and pair set readers — verification: tests with truncated coverage and pair set inputs do not panic
- [x] 2.3 Ignore a corrupt optional GPOS table and keep the font loadable; keep errors for required tables — verification: a font with a truncated GPOS parses and reports no kerning
- [x] 2.4 Add a fuzz seed with a truncated GPOS header — verification: `go test -fuzz` accepts the seed and 60 seconds of fuzzing reports no panic

## 3. Pagination progress (SEC-03)

- [x] 3.1 Detect a page that received neither drawing operations nor vertical progress and stop with `layout/no-progress` — verification: the review reproduction (`line-height: 100pt` on a 100pt page) fails with the diagnostic
- [x] 3.2 Return an error from `Convert` when layout reports no progress; write no PDF — verification: engine test asserts the error and the absence of output
- [x] 3.3 Turn the iteration cap into an error instead of a warning — verification: forced non-convergence test reports an error
- [x] 3.4 Keep normal multi-page documents unchanged — verification: the document corpus and the big table fixture still convert with the same page counts

## 4. Acceptance

- [x] 4.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate review-hardening` — verification: everything green, only the known language warnings on archived Russian specs
- [x] 4.2 Re-check the reviewed scenarios by hand: font graph, truncated GPOS, pagination reproduction — verification: each scenario produces the documented outcome
