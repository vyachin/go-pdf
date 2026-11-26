# Proposal

## Why

The package presents itself as library-first and writes nothing, but every asset it
reads must be an operating-system file. A caller that keeps templates and assets in
`embed.FS` has to extract them to a temporary directory before converting, which
adds a write and a deployment step the package otherwise avoids, and a caller that
generates an image or a font in memory cannot use it at all. There is no option
that accepts an `fs.FS`; the only inputs are `BaseDir`, `FontDir` and paths on the
host filesystem.

## What Changes

- Add `Options.Source fs.FS` (additive, nil by default). When it is set, every
  document-referenced image, stylesheet and font, and `Options.FallbackFont`, is
  read through it.
- `BaseDir` and `FontDir` name directories inside the source and default to its
  root; `Options.File` stays the diagnostic name and is not interpreted as a source
  path.
- A source path is relative, contains no `..`, and is validated before opening; an
  absolute or escaping reference is rejected with the same diagnostic a filesystem
  escape produces.
- The confinement, byte-limit, no-write and determinism guarantees apply
  unchanged: the same asset bytes produce a byte-identical PDF, and no file is
  created, modified or deleted.
- A missing or unreadable source asset produces the same diagnostic as the
  filesystem source (`img/load`, `css/link-load`, `font/unavailable`).
- A nil `Source` preserves today's behaviour exactly. The change is additive and
  does not require a migration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the asset-source requirement gains a caller-supplied `fs.FS` as an
  alternative to the host filesystem, with the same confinement, limits,
  diagnostics and determinism.

## Impact

- `pdfgen.Options` gains `Source fs.FS` (additive field, no new module dependency;
  `io/fs` is in the standard library).
- `internal/engine` (option plumbing and the explicit fallback font), `internal/assets`
  (a source-backed reader and its request), `internal/profile` (stylesheet reads).
- Package godoc, `README.md`, `docs/fonts.md`, `CHANGELOG.md`.
- Release note under `Added`; a nil source keeps every existing conversion
  identical, so no existing caller changes.
