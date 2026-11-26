# Design

## Context

`Options.RenderPNG` was added so tests and previews could see the first page
without an external rasterizer. It is implemented in `internal/engine` by calling
`internal/backend/raster`, encoding the image in memory and setting
`Result.PNG`. The 96 dpi value is a literal in `convert.go`; `MaxRasterPixels`
bounds the buffer; the engine imports the raster backend unconditionally. The
public godoc says only "meant for previews and tests", and the engine godoc says
"used by tests and the CLI debug flag" although the CLI has no such flag.
`png.Encode` writes into a `bytes.Buffer`, and the code ignores its error:
`if err := png.Encode(&buf, img); err == nil { pngData = buf.Bytes() }`.

The preview is part of the v2.0.0 option struct, and the package promises that
within 2.x a release does not remove what exists. Removal therefore cannot
happen in this change.

## Goals / Non-Goals

**Goals:**

- A truthful public contract for the preview: first page, 96 dpi, bounded.
- A diagnosed encoding failure instead of a silent empty result.
- A deprecation path that makes the eventual removal a major-release decision.
- Proof that producing the preview cannot change the PDF.

**Non-Goals:**

- Removing `Options.RenderPNG` now, or changing its type or default.
- Adding a new preview entry point or a `pdfgen/debug` package in this change.
- Changing the 96 dpi value or adding a DPI option.
- Removing the raster backend from the production dependency graph; that is the
  follow-up that unblocks once the field is gone.

## Decisions

**Keep the field and deprecate it, do not remove it in 2.x.** Removing it now
would break the stated compatibility promise and the tests that exercise the
preview, with no replacement in place. The godoc gains a `Deprecated:` paragraph
naming the next major release; the actual removal is a separate change after a
preview entry point exists. Alternative rejected: remove immediately — a
breaking change for a debug feature with no migration path.

**Do not add a `pdfgen/debug` or `pdfgen/preview` package now.** A preview package
needs either exported page geometry or a second pipeline call, and designing that
alongside a deprecation would freeze two surfaces at once. Alternative rejected:
move the option today — the new package would have to import `internal` code from
a public package or duplicate the engine wiring.

**Report an encoding failure as `raster/render`.** The existing code already uses
`raster/render` when the rasterizer fails; an encoding failure is the same
observable event: the requested preview could not be produced. Reusing the code
keeps the stable diagnostic set small and the position rule (the document, no
line) consistent. Alternative rejected: a new `raster/encode` code — it expands
the stable surface for a failure the caller cannot act on differently.

**Inject the encoder into an extracted helper.** `renderPreview(page, opts,
encode func(io.Writer, image.Image) error) ([]byte, error)` keeps the failure
path testable without a mutable package variable. The production call passes
`png.Encode`. Alternative rejected: a package-level encoder hook — it is mutable
global state on the conversion path, which the package's concurrency guarantee
forbids.

**Document 96 dpi as part of the contract.** A preview's pixel dimensions are
observable, so the number belongs in the godoc next to `MaxRasterPixels`.
Alternative rejected: add `RenderDPI` — it extends a deprecated option and makes
a debug detail a supported knob.

## Risks / Trade-offs

- **A caller depends on a silent nil preview** → surfacing an error is a
  behaviour change only on a path that previously lost the image; it is the
  correction the change exists for, and the release note is under `Changed`.
- **The removal never happens** → the `Deprecated:` marker and the migration
  section name the major release; the follow-up is recorded in the design and
  the changelog, not left as an implicit intention.
- **The new seam widens the internal API** → the helper is unexported and takes
  a standard function type; no public surface changes.
- **The raster dependency stays** → accepted for 2.x; the design states that
  removal of the option is the prerequisite for dropping it.

## Migration Plan

- **2.x:** the field works unchanged; godoc marks it deprecated and states the
  contract; an encoding failure now returns a `raster/render` error. Callers
  that only set the field keep compiling and producing the same PNG.
- **Next major (breaking):** remove `Options.RenderPNG` and `Result.PNG`, or
  replace them with the debug package chosen by then; drop the raster backend
  from the engine's production imports. The changelog entry names the removal and
  the replacement, and the migration section tells a caller to rasterize outside
  the library or adopt the new entry point.
