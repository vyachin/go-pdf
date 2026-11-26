# Tasks

## 1. Publish the contract and deprecation

- [x] 1.1 Rewrite the `Options.RenderPNG` godoc in `pdfgen/pdfgen.go` to state first page only, 96 dpi and `Limits.MaxRasterPixels`, and add a `Deprecated:` paragraph naming the next major release as the earliest removal. Verify with a source-level test (like `pdfgen`'s exported-docs test) that the comment contains "first page", "96 dpi", "MaxRasterPixels" and "Deprecated:".
- [x] 1.2 Correct the engine godoc in `internal/engine/engine.go`: it currently says the option is "used by tests and the CLI debug flag"; remove the CLI reference and mirror the public contract. Verify with `grep -n 'CLI debug' internal/engine` returning nothing and the source-level test in 1.1 covering the engine file as well.
- [x] 1.3 Add a `Deprecated` entry to `CHANGELOG.md` naming `Options.RenderPNG`, its contract and the major release that may remove it. Verify the entry names no other API and sits under an `Unreleased` heading.

## 2. Surface an encoding failure

- [x] 2.1 Extract `renderPreview(page, opts, encode func(io.Writer, image.Image) error) ([]byte, error)` from the `RenderPNG` block in `internal/engine/convert.go`; the production call passes `png.Encode`. Verify `go build ./...` and that the existing PNG test still produces a non-empty image.
- [x] 2.2 On an encoder error, add a `raster/render` error diagnostic naming the document (no line) and return the diagnostics error from `Convert`, leaving `Result.PNG` nil. Verify with a unit test that passes a failing encoder: the diagnostic carries `raster/render`, the returned error is not nil and `len(Result.PNG) == 0`.
- [x] 2.3 Keep and strengthen the byte-equality check: convert the same document with `RenderPNG` false and true and assert identical PDF bytes and identical diagnostics other than the preview. Verify with `go test ./internal/engine -run 'TestRenderPNG|TestPreviewDoesNotChangePDF'`.

## 3. Containment checks and follow-up record

- [x] 3.1 Add a no-preview test: with `RenderPNG` false, `Result.PNG` is nil and no `raster/render` diagnostic appears; with `Limits.MaxRasterPixels` below one page's raster, the conversion fails with `resource/limit`. Verify both through `pdfgen.Convert`.
- [x] 3.2 Run `go test -race ./internal/engine ./pdfgen` and confirm the preview path has no shared state; the test in 2.2 must pass under `-race`.
- [x] 3.3 Record the removal prerequisite in the change archive notes: `go list -deps ./cmd/pdfgen | grep backend/raster` still lists the backend, and it can only disappear together with the field. Verify the note names the field, the package and the next major release, and no code is changed for it here.

## Workflow follow-up

- Run `openspec validate refactor-renderpng-containment --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/library-api/spec.md` and archive the change after review; keep the deprecation in the changelog until the removal release.
