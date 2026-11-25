# Tasks

## 1. The encoding package

- [x] 1.1 Create `internal/charset` with `Resolve(src []byte, explicit string)`: resolve an explicit name through `charset.Lookup`, otherwise call `charset.DetermineEncoding`, accept UTF-8 (stripping a BOM) or one of the three single-byte encodings, and reject everything else — check: a table test covers the BOM, the declaration, the ASCII fallback, an undeclared non-ASCII document and an unsupported label
- [x] 1.2 Build an immutable 256-entry table per encoding from the Go decoder: the UTF-8 bytes of every byte's rune, and the bytes the encoding leaves undefined — check: every byte of every supported encoding is covered; windows-1251 0x98 is the only undefined byte in the set
- [x] 1.3 Decode a document with the table, returning the decoded text, the width of every source byte and the source line starts; report the first undefined byte — check: a cp1251 `Счёт` decodes to the expected runes and widths
- [x] 1.4 Share encodings and tables across conversions without mutable state — check: `-race` with two conversions in different encodings

## 2. Positions in the original file

- [x] 2.1 Implement `Mapper.Position(decodedOffset)` returning the line and the byte column of the source, with a one-line cache — check: a table test over ASCII, Cyrillic and mixed lines, CRLF and empty lines
- [x] 2.2 Add `Charset` to `html.ParseOptions`, decode before the scan, keep the original bytes on `Document`, and translate in `Document.Position` — check: UTF-8 tests are unchanged; a cp1251 test returns the source column
- [x] 2.3 Keep inline `<style>` positions on `doc.Position` — check: a diagnostic for a cp1251 inline sheet names the source line and column

## 3. Diagnostics and the public surface

- [x] 3.1 Return `html/encoding` for an undecodable document, an undefined byte and an unsupported declaration, with the position rules from the design — check: the four scenarios of the spec, including the UTF-16 BOM
- [x] 3.2 Add `Options.Charset` to `pdfgen` and `engine`, validated as `options/charset` before any file is read — check: an unknown value fails with the code and no asset is opened
- [x] 3.3 Add `-charset` to `cmd/pdfgen`; an unknown value prints the usage line and exits `2` — check: the CLI test checks the code, the message and the absence of an output file
- [x] 3.4 Strip a UTF-8 BOM before parsing — check: the BOM fixture and the same document without it produce byte-identical PDFs

## 4. External stylesheets

- [x] 4.1 Decode a linked sheet with the document's encoding and pass a `Locate` closure over its mapper to `css.ParseSheet` — check: a cp1251 sheet applies, and its diagnostic names the source position
- [x] 4.2 Consume `@charset` at offset 0: accept it when it matches, fail with `css/link-encoding` when it conflicts or the file has an undefined byte — check: the three spec scenarios
- [x] 4.3 Leave a UTF-8 document with a UTF-8 sheet unchanged — check: the existing external-stylesheet tests are green without edits

## 5. Fixtures and tests

- [x] 5.1 Add binary fixtures: a cp1251 invoice with the `http-equiv` declaration, a KOI8-R document, a UTF-8 BOM document and a cp1251 linked sheet — check: `.gitattributes` marks them `-text` and the declared encoding matches the bytes
- [x] 5.2 Freeze the cp1251 invoice in the golden layers and verify the text layer through the ToUnicode map — check: two runs are byte identical and `pdfcpu` strict mode passes
- [x] 5.3 Cover detection: BOM, `<meta charset>`, the `http-equiv` form, a declaration inside a comment, the 1024-byte boundary, no declaration — check: every case from the spec and the design table
- [x] 5.4 Cover hostile input: invalid UTF-8, an undefined byte, a declaration cut at the boundary and an 8 MiB single-line cp1251 document with many diagnostics — check: no panic, bounded time, the expected codes
- [x] 5.5 Cover determinism and concurrency: repeated conversions are byte identical, parallel conversions in different encodings agree with sequential ones — check: `go test -race ./internal/engine`

## 6. Documentation

- [x] 6.1 README: an input-encoding section (supported set, precedence, the override, positions) and the dependency sentence without a module count — check: the documented example converts the cp1251 fixture
- [x] 6.2 `docs/profile.md`: the `meta` declaration, the closed set, external stylesheet encoding and `@charset` — check: the section matches the CLI behavior
- [x] 6.3 CHANGELOG: the new capability and diagnostics under Added, the dependency change under Changed — check: entries in the Unreleased section
- [x] 6.4 CONTRIBUTING: the fixtures are binary and the `.gitattributes` rule is explained — check: the rule is present

## 7. Acceptance

- [x] 7.1 `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` — check: green
- [x] 7.2 The pinned `golangci-lint` run — check: green
- [x] 7.3 `openspec validate legacy-input-encodings` — check: valid
