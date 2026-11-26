# Tasks

## 1. Correct transparent image samples

- [ ] 1.1 In `internal/pdf/png.go`, unpremultiply palette entries and gray+alpha samples before appending the colour bytes, leaving the alpha stream and `/SMask` untouched. Verify with a table test that decodes the generated image stream and asserts `ff 00 00` + mask `80` for a palette PNG (`PLTE=(255,0,0)`, `tRNS=128`) and `80` + `80` for a gray+alpha PNG (128/128).
- [ ] 1.2 Add an opaque control case for palette and NRGBA images: identical colour bytes to the source and no `/SMask`. Verify the test fails when the unpremultiply step is removed and passes here.
- [ ] 1.3 Run `go test ./internal/pdf ./internal/engine` and confirm only the intentionally regenerated image goldens move; inspect the PDF image stream of one regenerated file with `pdfdebug` to confirm the bytes.

## 2. Font descriptor and text layer

- [ ] 2.1 In `internal/pdf/writer.go`, compute `/Flags` as `4` plus bit 7 (64) when `fe.font.ItalicAngle() != 0`; write the same angle to `/ItalicAngle`. Verify with a test that loads an italic test face and asserts `Flags 68`, and an upright face asserting `Flags 4`.
- [ ] 2.2 Include every emitted CID in `toUnicodeCMap`, mapping rune U+0000 to `<0000>`. Verify with a writer-level test that a run containing a NUL-rune glyph produces one `bfchar` per CID and no missing entry; assert a normal run is byte-identical to the previous output.
- [ ] 2.3 Run `go test ./internal/pdf -run 'TestFont|TestToUnicode|TestSubset'` and `pdfcpu validate -m strict` on a generated document whose font is italic.

## 3. Stream compression

- [ ] 3.1 Refactor `streamObject` so the zlib writer is closed on every path and its `Close` error is handled; on write/close failure or no size saving, return the uncompressed body. Verify with a seam test that injects a writer returning an error and asserts the uncompressed body, the correct `/Length`, and that close was called once.
- [ ] 3.2 Keep the small-data compression test: a compressible stream still carries `/Filter /FlateDecode` and the compressed length. Verify `pdfcpu validate -m strict` on a multi-page document and `go test ./internal/pdf`.
- [ ] 3.3 Add a fuzz seed for a stream object whose input is random bytes, so close-on-every-path stays exercised. Verify with `go test ./internal/pdf -fuzz '^FuzzWriter$' -fuzztime 10s` (or the existing writer fuzz target).

## Workflow follow-up

- Run `openspec validate fix-pdf-output-defects --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/pdf-output/spec.md` and archive the change after review.
