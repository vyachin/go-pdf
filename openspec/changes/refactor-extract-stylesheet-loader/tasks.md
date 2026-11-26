# Tasks

## 1. Create the stylesheet stage

- [x] 1.1 Add `internal/stylesheet` with `Options`, `Collect(doc, opts)` and `CollectInline(doc, opts)`, moving `loadStylesheet`, `decodeSheet`, `charsetRule`, `isCSSSpace` and `stylesheetReadError` from `internal/profile/styles.go` and renaming the internal identifiers to the package. Verify with `go build ./...` and by running the moved charset/link tests in the new package.
- [x] 1.2 Switch `internal/engine/convert.go` to call `stylesheet.Collect`/`stylesheet.CollectInline` and keep `profile.ValidateSheets` after collection in the same order. Verify with `go test ./internal/engine ./internal/profile ./internal/stylesheet` and a byte comparison of one converted document before and after.
- [x] 1.3 Keep `ParseFontFaceSrc` in profile as descriptor parsing, and move only the tests that exercise file loading or decoding. Verify `go test ./internal/profile` still covers the allow-list, generated content and `@font-face` descriptor rules.

## 2. Remove the duplicate resolver

- [x] 2.1 Delete `profile.resolveStylesheetPath` and let `assets.Registry.Read` be the only resolver; map `*assets.Error` and `fsx.ErrOutsideRoot` to `css/link-load`, `css/link-path`, `css/link-remote` and `css/link-unsupported` in the loader. Verify with a test that an href with `?v=1`, `#frag`, `%20` and surrounding spaces reads one file and produces the same path as before.
- [x] 2.2 Add a table-driven diagnostic matrix for a missing file, a remote URL, a `data:` URI, an escaping path, a directory and an oversized sheet, asserting code, message prefix, file and position. Verify each row against the pre-change output captured from `main`.
- [x] 2.3 Verify a legacy-encoded sheet and an `@charset`-carrying sheet keep their line and column mapping: run the moved encoding tests and one converted document each from windows-1251 and KOI8-R.

## 3. Enforce and document the boundary

- [x] 3.1 Add `internal/profile/imports_test.go` that parses the package's production files with `go/parser` and fails when `internal/assets`, `internal/fsx`, `internal/asset`, `os` or `path/filepath` appears. Verify the test fails on the parent commit and passes here.
- [x] 3.2 Update the layout section of `CONTRIBUTING.md` to show `internal/stylesheet` between the engine and the asset registry, and state that `internal/profile` validates only. Verify the diagram names every package on the stylesheet path and no other document contradicts it (`grep -rn 'profile' CONTRIBUTING.md docs/`).
- [x] 3.3 Run `go test -shuffle=on ./...`, `go vet ./...` and `gofmt -l $(git ls-files '*.go')`; confirm the golden document and render snapshots do not move.

## Workflow follow-up

- Run `openspec validate refactor-extract-stylesheet-loader --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/code-quality/spec.md` and archive the change after review.
