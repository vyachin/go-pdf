# Tasks

## 1. Streaming serialiser

- [ ] 1.1 Add `pdf.Doc.WriteTo(w io.Writer) (int64, error)` that writes the header, page content and page objects, then fonts, images, outline, catalog and info, then the xref and trailer, recording offsets from a counting writer. Verify with `pdfcpu validate -m strict` on the produced file and with the existing object-level tests.
- [ ] 1.2 Implement `Doc.Bytes` as `WriteTo` into a `bytes.Buffer`, and add a test that `Bytes` and `WriteTo` produce identical bytes for a corpus document with images, fonts and an outline. Verify the test fails if either path is changed alone.
- [ ] 1.3 Add the write-error and short-write paths: one `writeAll` helper, an error returned from `WriteTo`, and no panic. Verify with a writer that fails after N bytes and one that returns a short write without an error.

## 2. Public API and engine

- [ ] 2.1 Add `ConvertTo(ctx, dst io.Writer, src []byte, opts Options) (Result, error)` in `pdfgen`, implement it and `Convert` over one engine sink, and set `Result.PDF` to nil only in the streaming path. Verify the package documentation test (`TestExportedDocs`) passes and the godoc states the partial-output contract.
- [ ] 2.2 Check the context between pages and before the xref, and map a write error to `pdf/write` while keeping a context error ahead of diagnostics. Verify with a cancelled context during a many-page document and with a failing writer.
- [ ] 2.3 Add the `README.md` streaming example and the `CHANGELOG.md` `Unreleased / Added` entry, naming the nil-`PDF` contract and the partial-output rule. Verify the README example compiles as an `Example` test.

## 3. Equivalence and memory

- [ ] 3.1 Add a test that converts the same document with `Convert` and `ConvertTo` into a buffer and compares bytes and diagnostics. Verify it covers a document with an image, an @font-face font, an outline and a page break.
- [ ] 3.2 Add a many-page benchmark measuring peak heap for `Convert` against `ConvertTo` to `io.Discard`, and assert the streamed heap is lower by at least the PDF size and does not grow with page count beyond one page's operations. Verify with `go test -run '^$' -bench Streaming`.
- [ ] 3.3 Run the full suite, the `-race` job, the golden render check and `PDF_REQUIRE_VALIDATOR=1 go test ./...`; confirm no existing PDF bytes, render goldens or diagnostic lists change except the intended object order.
