# Tasks

## 1. Public option and source path model

- [x] 1.1 Add `Source fs.FS` to `pdfgen.Options` and its engine mirror, document that a nil value keeps the filesystem, and keep the zero value a complete request. Verify with `go test ./pdfgen -run 'TestDefaultOptions|TestExportedDocs'`.
- [x] 1.2 Resolve `BaseDir`/`FontDir` as source paths (default the source root), clean a reference with `path.Clean`, require `fs.ValidPath` and reject an absolute name or a remaining `..`. Verify with a table test for `logo.png`, `sub/logo.png`, `../secret.png`, `/etc/hosts` and an empty `BaseDir`.

## 2. Source-backed asset reader

- [x] 2.1 Read images, stylesheets and `@font-face` sources through `fs.Open` and `io.LimitReader` with the per-file and total limits, and route `FallbackFont` through the same reader when `Source` is set. Verify an `embed.FS` document with an image and a font converts.
- [x] 2.2 Keep the diagnostics identical to the filesystem source for a missing image, stylesheet and font, and for a rejected path. Verify the codes `img/load`, `css/link-load` and `font/unavailable` with `fstest.MapFS`.
- [x] 2.3 Prove determinism and no writes: convert the same document from `os.DirFS` and from the host filesystem and compare the PDF bytes; assert the working directory is unchanged. Verify with a test that records the directory entries before and after.

## 3. Documentation and release note

- [x] 3.1 Document `Source` in the package godoc, `README.md` and `docs/fonts.md`: it replaces the host filesystem for the conversion, `BaseDir`/`FontDir` are paths inside it, `FallbackFont` comes from it, and no source is mixed with host paths. Verify the docs test and that every statement has a matching scenario.
- [x] 3.2 Add an `Added` entry to `CHANGELOG.md` naming `Options.Source` and the nil-source compatibility. Verify the entry is not marked breaking and names no removed or renamed identifier.

## Workflow follow-up

- Run `openspec validate add-asset-source-interface --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/library-api/spec.md` and archive the change after review.
