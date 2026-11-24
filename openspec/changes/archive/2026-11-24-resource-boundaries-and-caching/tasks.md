# Tasks

## 1. Stylesheet boundary through symlinks (SEC-02)

- [x] 1.1 Open stylesheets through `os.OpenRoot` when a base directory is set, keeping lexical diagnostics for `..` and absolute escapes — verification: test with a symlink pointing outside fails with `css/link-path`
- [x] 1.2 Accept symlinks that stay inside the base — verification: test with an internal symlink loads the stylesheet and applies its rule
- [x] 1.3 Keep the no-base-directory behaviour — verification: existing `css/link-*` tests pass

## 2. Per-page image resources

- [x] 2.1 Emit `/XObject` entries only for images the page draws, in document order — verification: a two-page document lists no image on the page that does not draw it
- [x] 2.2 Keep determinism and ordering — verification: repeated serialisation test and first-use order test still pass

## 3. One read per conversion (ENG-03)

- [x] 3.1 Split validation into collection and validation of collected sheets; `Convert` collects once and reuses the slice — verification: conversion reads a linked stylesheet once (asserted by a counter loader or a single decode path)
- [x] 3.2 Cache decoded images per source for the conversion so a repeated `<img>` shares one `ImageData` — verification: a document with the same image twice contains one image object
- [x] 3.3 Keep corpora green — verification: document corpus and mpDF fixtures pass with unchanged page counts and text

## 4. Readability

- [x] 4.1 Add a comment to `advanceAdjustment` with the `[<0041> 42 <0056>] TJ` example and the sign rule — verification: comment present, no behaviour change

## 5. Acceptance

- [x] 5.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate resource-boundaries-and-caching` — verification: everything green
- [x] 5.2 Confirm the boundary by hand: symlink out is rejected, symlink in works, page resources shrink — verification: test output and a generated PDF inspected
