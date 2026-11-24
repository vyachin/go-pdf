# Tasks

## 1. Deterministic image resources (PDF-02)

- [x] 1.1 Write `/XObject` resources by iterating the ordered image slice instead of the name map — verification: unit test serialises a document with three images thirty times and compares bytes
- [x] 1.2 Assert that resource entries follow first-use order — verification: test checks the position of each name in the page dictionary

## 2. Kerning in the PDF (PDF-03)

- [x] 2.1 Add `Advance` to the PDF glyph type and copy it in `engine.toPDFOps` — verification: engine test inspects the produced ops for a nonzero advance
- [x] 2.2 Emit text runs as `TJ` arrays with adjustments computed from the nominal `/W` width — verification: writer unit test pins the sign and magnitude of the adjustment
- [x] 2.3 Verify against a real font: a kerned pair produces a nonzero adjustment and the effective advance matches layout within one thousandth of an em — verification: engine test with the test font

## 3. Reader compatibility

- [x] 3.1 Read `TJ` arrays in `internal/pdfdebug` while keeping `Tj` support — verification: extraction returns the same characters for the document corpus and the mpDF fixtures
- [x] 3.2 Update the content stream operator test to accept `TJ` — verification: `go test ./internal/engine` passes

## 4. Acceptance

- [x] 4.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate pdf-kerning-and-image-order` — verification: everything green
- [x] 4.2 Confirm reproducibility end to end: the same template twice gives identical files, and a template with two images does too — verification: command output compared with `shasum`
