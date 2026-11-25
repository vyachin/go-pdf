# Tasks

## 1. Memory and work bounds (RV4-01 … RV4-07)

- [x] 1.1 RV4-01: cap declared GPOS pairs per font, charge each PairSet before its inner loop and charge coverage format 2 by expansion — verification: a 264 KB pair-bomb font fails with `resource/limit` under the default limits; the existing `p1-crafted-gpos` fixture still passes
- [x] 1.2 RV4-02: thread the guard into `css.Tokenize` and the parser, charge tokens and cap stored problems per sheet with one `resource/limit` — verification: a 1 MiB stylesheet of `}` returns a bounded diagnostic list and bounded memory
- [x] 1.3 RV4-03: cache built lines per box and width, reuse them in `measure`, and charge every built line — verification: 240 nested `break-inside: avoid` blocks convert in linear time and a small `MaxOperations` stops them quickly
- [x] 1.4 RV4-04: charge `len(runes)` in shaping and line building and drop the per-rune `states` slice — verification: a 2 MB word with `MaxOperations: 100000` fails before allocating out of proportion to the budget
- [x] 1.5 RV4-05: pass the guard into `convertCFF`, charge per glyph and instruction, check the guard between glyphs and add a font-wide instruction cap — verification: the 459 KB CFF bomb stops with `resource/limit`, and cancellation interrupts it
- [x] 1.6 RV4-06: charge serialised bytes, glyphs, CIDs and decoded pixels in the writer and in `assemble` — verification: a 9000-CID document shows a spend proportional to its 3.86 MB output and a small budget stops it
- [x] 1.7 RV4-07: index the previous element or charge per visited sibling in adjacent-sibling matching — verification: 80000 siblings with `p + p` stay close to `p p`, and a small budget stops the conversion
- [x] 1.8 Register every hostile case above as a fixture under `testdata/adversarial/` and in `adversarialCases()` — verification: `TestAdversarialManifest` passes and each fixture asserts a diagnostic code or a bounded output, never elapsed time

## 2. Font and text (RV4-31, RV4-40 … RV4-44)

- [x] 2.1 RV4-31: keep U+00A0 in the glyph run and the `ToUnicode` map — verification: `pdftotext` of `a&nbsp;b` contains U+00A0 and the line does not break there
- [x] 2.2 RV4-40: honour `ARGS_ARE_XY_VALUES` in composite placement, rejecting point matching or implementing it — verification: a point-matching composite is rejected or placed by its points, never by the raw indices
- [x] 2.3 RV4-41: return an error for a glyph past the composite depth cap and keep every component in the subset — verification: a twelve-deep chain renders or fails with a diagnostic, and the subset contains the used glyph's components
- [x] 2.4 RV4-42: select GPOS lookups through `ScriptList`, `FeatureList` and `lookupFlag`, honouring `IgnoreMarks` — verification: a non-default-script `PairPos` lookup does not affect ordinary text
- [x] 2.5 RV4-43: key the face cache on an unambiguous family list — verification: `"a b"` and `a, b` resolve to their own faces in one conversion
- [x] 2.6 RV4-44: charge system-fallback parsing like `@font-face` parsing — verification: an exhausted budget stops the fallback load with `resource/limit`

## 3. Contract and diagnostics (RV4-09, RV4-28, RV4-37, RV4-59, RV4-62)

- [x] 3.1 RV4-09: move metadata validation into `prepare`, document serialisation as the only stage `Validate` misses and assert the shared diagnostics — verification: an invalid title fails both entry points with `options/title`
- [x] 3.2 RV4-28: validate inline `<style>` rules when an option is invalid without opening linked assets — verification: `PageSize: "banana"` plus a bad `<style>` declaration reports both codes
- [x] 3.3 RV4-37: carry the `src` declaration position into `font/unavailable` — verification: a missing font names the document, line and column
- [x] 3.4 RV4-59: make `CharsetSupported("")` true and document the guard for callers — verification: the empty name reports true and `shift_jis` stays false
- [x] 3.5 RV4-62: stop dependent diagnostics after a budget failure — verification: an exhausted font budget reports `resource/limit` without a second `text/font` error

## 4. Positions and encodings (RV4-10, RV4-17, RV4-38, RV4-39)

- [x] 4.1 RV4-10: record a node's span from its own token, or verify a sliced span against the node text before using it — verification: a `<style>` inside a `<table>` keeps `p{color:red}` and its positions
- [x] 4.2 RV4-17: distinguish a declared windows-1252 from the no-declaration fallback — verification: `<meta charset="windows-1252">` over valid UTF-8 fails with `html/encoding`
- [x] 4.3 RV4-38: fold ASCII only when locating the `<meta>` declaration — verification: a declaration after a multi-byte word is reported at its byte offset
- [x] 4.4 RV4-39: define the column contract (byte or character), implement it for both encodings, document and pin it — verification: one test covers a UTF-8 line after Cyrillic and a legacy line after Cyrillic

## 5. CSS grammar and matching (RV4-11, RV4-18 … RV4-20, RV4-36, RV4-46 … RV4-50, RV4-61)

- [x] 5.1 RV4-11: reject CSS-wide keywords in `font-family` or implement `inherit` — verification: `font-family:inherit` inherits or fails, and never selects a system font
- [x] 5.2 RV4-18: accept `media` on `<style>` and share the media-list parser — verification: a print-only `<style>` validates under `Strict`
- [x] 5.3 RV4-19: reject a pseudo-element outside the final compound — verification: `a::after span` fails with `css/unsupported-selector` and does not style the span
- [x] 5.4 RV4-20: reject or resolve a percentage `height` — verification: `height:50%` resolves or fails, never silently behaves as `auto`
- [x] 5.5 RV4-36: resolve a relative path against the root directory and stat through the handle — verification: `Root.Read("sub/file")` works from any working directory
- [x] 5.6 RV4-46: report a nested at-rule the parent cannot represent and reject `@page` inside `@page` — verification: a media rule inside `@font-face` reports a problem
- [x] 5.7 RV4-47: reject a second id in a compound or require every id to match and count — verification: `#x#y` does not match `<p id="y">`
- [x] 5.8 RV4-48: decode escapes in ident and hash tokens — verification: `caf\e9` names `café` in a family and a selector
- [x] 5.9 RV4-49: use one media-list parser in validation and collection — verification: `@media print, screen` applies for print
- [x] 5.10 RV4-50: validate against the resolved parent size and never drop a declaration silently — verification: a font size that would round to zero fails validation
- [x] 5.11 RV4-61: document `always` as an alias of `page` — verification: a test asserts the parser and the property table agree

## 6. Layout and pagination (RV4-12 … RV4-16, RV4-21, RV4-22, RV4-32 … RV4-35)

- [x] 6.1 RV4-12: require a box to fit before a `break-inside: avoid` transition and honour a refused page — verification: a 60-line avoid block produces no empty first page
- [x] 6.2 RV4-13: clip a spanning cell to the page and paint its continuation — verification: a `rowspan=60` cell keeps its text inside the page and continues on the next
- [x] 6.3 RV4-14: consume a declared height across fragments — verification: a 400 pt box split across pages reserves 400 pt in total
- [x] 6.4 RV4-15: clamp or scale table columns and warn once — verification: a 1000 pt cell does not push the next cell off the page silently
- [x] 6.5 RV4-16: report `layout/overflow` when an unbreakable token cannot fit, or break it — verification: the 1000-character word and the CJK paragraph emit the warning
- [x] 6.6 RV4-21: detect the image format from magic bytes — verification: a PNG named `.jpg` or without an extension loads
- [x] 6.7 RV4-22: resolve asset references as URL paths — verification: `logo.png?v=2`, `#top`, `%20` and leading whitespace resolve to the named file
- [x] 6.8 RV4-32: use saturating arithmetic and clamp the content box — verification: a margin larger than the page produces no wrapped coordinates
- [x] 6.9 RV4-33: compute image area in 64 bits — verification: a 32-bit build rejects an image whose area overflows `int`
- [x] 6.10 RV4-34: commit the aggregate image totals only on success — verification: a corrupt image does not reject a later valid one
- [x] 6.11 RV4-35: require a JPEG end-of-image, or decode it within the pixel budget — verification: a truncated JPEG fails in both entry points

## 7. PDF output (RV4-23, RV4-24, RV4-51 … RV4-54, RV4-66)

- [x] 7.1 RV4-23: validate the UTC year written to `/CreationDate` — verification: a local 9999 date with a negative offset fails with `options/date`
- [x] 7.2 RV4-24: preserve PNG colour type and bit depth — verification: a 16-bit gray PNG keeps 16-bit samples and a 1-bit image is not expanded to RGB
- [x] 7.3 RV4-51: pack contiguous glyphs and split a `TJ` at 8191 elements — verification: a 20000-character token produces several bounded arrays and unchanged text
- [x] 7.4 RV4-52: split `/W` into `startCID` groups — verification: a 9000-CID subset emits several groups
- [x] 7.5 RV4-53: skip an image op with a non-positive width or height — verification: a zero-height image writes no `cm`
- [x] 7.6 RV4-54: cap the subset tag plus family name at 127 bytes — verification: a 200-character family name yields a bounded `/BaseFont` with a distinct tag per subset
- [x] 7.7 RV4-66: decode a PNG once and use typed fast paths — verification: a repeated PNG is decoded once per conversion

## 8. Documentation and process (RV4-08, RV4-25 … RV4-27, RV4-29, RV4-30, RV4-45, RV4-55 … RV4-58, RV4-60, RV4-63 … RV4-65)

- [x] 8.1 RV4-08: release the removal of `Options.ImportBrowser` as v2.0.0 with migration notes and state the documented version — verification: the changelog and the README agree, and the tag is a major
- [x] 8.2 RV4-25: complete the diagnostic catalogue, preferably generated, with a code test — verification: every emitted code is documented and every documented code is emitted
- [x] 8.3 RV4-26: remove the browser-lowering claim from the README — verification: `doc.go` and the README agree that there is no lowering pass
- [x] 8.4 RV4-27: align `docs/profile.md` with the accepted generated content and state the position rule — verification: the documented `a::after` and margin-box forms validate
- [x] 8.5 RV4-29: pin the golden image in the workflow, or change the docs and the spec, and reconcile `renderer.txt` — verification: workflow, documentation and the renderer record name the same image and poppler
- [x] 8.6 RV4-30: validate `startxref`, xref and `/Length` in the extractor and add a glyph-level or render check — verification: a PDF with `/Length` off by seven fails the suite
- [x] 8.7 RV4-45: widen the sfnt oracle to converted CFF fonts or narrow its claim — verification: the documented claim and the test agree, and no glyph is silently skipped
- [x] 8.8 RV4-55: document the guard's concurrency contract and remove the stale mutex sentence — verification: the package documentation matches the implementation
- [x] 8.9 RV4-56: remove dead internal API and stale consumer comments — verification: `go vet` plus a dead-code check, and no comment names the removed CLI or importer
- [x] 8.10 RV4-57: remove the environment hook from the production path and move the override behind a test seam — verification: a conversion with no font option ignores the environment
- [x] 8.11 RV4-58: move the test-font lookup into one internal test helper — verification: every package uses the shared helper
- [x] 8.12 RV4-60: document the `FontDir` fallback and the `options/asset-root` diagnostic — verification: the godoc of `Options` states both
- [x] 8.13 RV4-63: fix the broken list, the command fence and the removed-flag references — verification: a render or a docs check shows commands and prose in their own blocks
- [x] 8.14 RV4-64: give open work one home — verification: every documented gap is an active change or a known-gaps entry, and `TODO.md` says so
- [x] 8.15 RV4-65: refresh the benchmark figures or label them indicative with the command — verification: the section matches a recent run

## 9. Improvements (IMP-01 … IMP-12)

- [x] 9.1 IMP-01: declare every diagnostic code in one `internal/diag` registry, generate the catalogue and add the registry test — verification: a raw code literal fails the test and the documentation table is generated
- [x] 9.2 IMP-02: serve `Convert` and `Validate` from one pipeline with only the output stage differing — verification: a new preparation check reaches both entry points without a second test fixture
- [x] 9.3 IMP-03: document what one unit means per stage and add the invariant test for each adversarial fixture — verification: a stage that allocates without charging fails the invariant test
- [x] 9.4 IMP-04: record source spans during tree construction and inherit an ancestor range for implied nodes — verification: a new raw-text kind gets its own span and the foster-parenting test stays green
- [x] 9.5 IMP-05: resolve every asset kind through one resolver — verification: images, stylesheets and fonts share scheme, query, percent and format handling
- [x] 9.6 IMP-06: generate or check the code facts a document states (limits, codes, CI image, renderer) — verification: changing a default fails the documentation test until the document follows
- [x] 9.7 IMP-07: dropped from the change — the release check and its CI job were removed at the author's request, and the IMP-07 requirement was withdrawn from the specs
- [x] 9.8 IMP-08: run an independent writer validator, not only the debug extractor — verification: a PDF with a wrong `/Length` or shifted xref fails the suite
- [x] 9.9 IMP-09: add the coverage floor and require the validator on every job that writes a PDF — verification: a skipped required check and a below-floor run both fail the gate
- [x] 9.10 IMP-10: move the test-support code into one internal test package and remove the production environment hook — verification: every package uses the shared helper and a conversion ignores the environment
- [x] 9.11 IMP-11: remove each hot spot the review measured and add the benchmark — verification: the benchmark reports the reduction in allocations or time
- [x] 9.12 IMP-12: share the media-list, escape, keyword and nested-at-rule implementations and document each policy — verification: validation and collection agree on a media list and an escaped name

## 10. Acceptance

- [x] 10.1 Run `gofmt -l`, `go vet ./...`, `golangci-lint run ./...`, `go test -shuffle=on ./...`, `go test -race -shuffle=on ./...` and `PDF_REQUIRE_VALIDATOR=1 go test ./...` — verification: everything green
- [x] 10.2 Run the snapshot layers and review every changed golden — verification: the document snapshots, the render snapshots and the corpus updates are one reviewed commit
- [x] 10.3 Re-read every `RV4-` and `IMP-` entry in `TODO.md` against its requirement and its test — verification: each of the 66 findings and 12 proposals is either closed by a task above or explicitly deferred with a reason
- [x] 10.4 Run `openspec validate fourth-review-hardening --strict` and, after implementation, `openspec archive` — verification: the change validates and archives cleanly (archived 2026-11-25)
