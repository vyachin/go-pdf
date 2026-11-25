# Tasks

## 1. Normalization

- [x] 1.1 Replace the body of `text.Normalize` with `norm.NFC.String`, keep the wrapper and its documentation, delete `HasCombiningMark` — check: `TestNormalize` and the combining-class ordering test pass without edits
- [x] 1.2 Delete `internal/text/unicode_nfc.go` and `internal/text/gen_nfc.py`, and the `combiningClasses` table — check: `go build ./...` and no reference remains
- [x] 1.3 Add the case the private code missed (`U+1E0B U+0323` → `U+1E0D U+0307`) and one non-BMP case — check: the new table entries pass
- [x] 1.4 Check the corpus and both golden layers for changed text — check: `go test ./...` is green; a regenerated golden is explained in the commit and the changelog
- [x] 1.5 Confirm normalization did not get slower — check: `go test ./internal/engine -bench .` stays within noise of the previous run

## 2. JPEG headers

- [x] 2.1 Read width, height and components with `jpeg.DecodeConfig` in `imageObject` and delete `jpegInfo` — check: a grayscale, an RGB and a CMYK fixture each get the right `/ColorSpace`, and CMYK is still rejected
- [x] 2.2 Fail with a diagnostic for a JPEG the standard library cannot decode — check: an unsupported variant produces an error and no image object
- [x] 2.3 Keep the existing image and PDF tests green — check: `go test ./internal/engine ./internal/pdf`

## 3. UTF-16

- [x] 3.1 Encode with `utf16.Encode` in `utf16Hex` — check: BMP output is byte identical and a non-BMP character keeps its pair
- [x] 3.2 Decode with `utf16.Decode` in `decodeUTF16Hex` — check: a surrogate pair extracts as one rune; a regression test covers it
- [x] 3.3 Verify ToUnicode and outline strings are unchanged — check: `go test ./internal/pdf ./internal/pdfdebug ./internal/engine`

## 4. The standard-library sweep

- [x] 4.1 Replace `maxInt` with the builtin `max` — check: `go vet ./...`
- [x] 4.2 Use `slices.SortStableFunc` in `diag` and `css` — check: the diagnostic order and cascade tests are green
- [x] 4.3 Use `slices.SortFunc` elsewhere and `slices.Sorted(maps.Keys(m))` for map keys — check: the docdump snapshot and the golden order are unchanged
- [x] 4.4 Use `slices.BinarySearch` for the line lookup in `css` and `html` — check: the position tests are green
- [x] 4.5 Use `strconv.Quote` and `fmt`'s `%04X`, delete `strconvQuote` and `hex4` — check: the missing-glyph message test is green
- [x] 4.6 Sweep the remaining packages for a private substitute — check: the diff is reviewed and anything left is added to the exception list

## 5. One containment rule

- [x] 5.1 Drop the `filepath.Abs`/`Rel` boundary check from `resolveStylesheetPath`, keep the URL rules and the join with `BaseDir` — check: `../../outside.css` still yields `css/link-path` and the file is not read
- [x] 5.2 Confirm the search behaves like the image loader — check: the symlink test, and the missing, oversized and outside cases report the same codes

## 6. Policy and documentation

- [x] 6.1 README: one sentence that the Go project's implementation is used where one exists; no module count — check: the sentence is present and no count remains
- [x] 6.2 CONTRIBUTING: the exception list with a reason for each (font subsetter, debug raster, CSS grammar, PDF syntax, browser-lowering case mapping) — check: every exception from the design is listed
- [x] 6.3 CHANGELOG: the normalization, JPEG and UTF-16 changes under Changed, the surrogate fix under Fixed — check: entries in the Unreleased section
- [x] 6.4 Confirm the repository matches the `code-quality` requirement — check: no private substitute remains outside the recorded exceptions

## 7. Acceptance

- [x] 7.1 `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` — check: green
- [x] 7.2 The pinned `golangci-lint` run — check: green
- [x] 7.3 `openspec validate go-core-first` — check: valid
