# Proposal

## Why

With `BaseDir` and `FontDir` unset, an absolute asset reference is treated as an
operating-system path and opened as written: `/etc/hosts` is read as an image, a
stylesheet and a font. The package documentation says that when both options are
empty "the process working directory is used", which reads as confinement but
describes only how a relative reference is resolved. A server that converts an
untrusted template with the zero `Options` value therefore reads any file the
process can read, and the README guarantee that assets stay "inside the configured
roots" has no root to apply.

## What Changes

- A relative image or stylesheet reference resolves against `BaseDir`; a font
  reference resolves against `FontDir`, then `BaseDir`.
- When those roots are empty, the root is the directory of `Options.File`; when
  `File` is empty as well, it is the process working directory.
- Every relative reference is confined to its root: a `..` path or a symlink that
  leaves the root is rejected, including when the root is the working directory.
- An absolute reference is accepted only when it lies inside a configured root.
  With no configured root it is rejected unless the caller opts in with the new
  `Options.AllowAbsoluteAssets`.
- **BREAKING**: an absolute reference with no configured root is no longer read as
  written, and a relative asset under an absolute `Options.File` now resolves
  against the document's directory instead of the process working directory.
- **BREAKING**: an absolute `@font-face` source is now confined to `FontDir` even
  when `FontDir` is set; previously fonts were always read as written.
- The package documentation, README and `docs/fonts.md` state the enforced rule.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the asset-root requirement becomes confinement-by-default, with
  the `Options.File` directory as the fallback root and an explicit opt-in for
  absolute references.

## Impact

- `pdfgen.Options` gains `AllowAbsoluteAssets bool` (additive field, default
  false, safe to leave unset).
- `internal/engine` (option plumbing), `internal/assets` and `internal/fsx`
  (default root and boundary), `internal/profile` (stylesheet root).
- `README.md`, `pdfgen` package documentation, `docs/fonts.md`, `CHANGELOG.md`.
- Migration for a caller that relied on the old behaviour: configure `BaseDir` /
  `FontDir`, or set `AllowAbsoluteAssets: true`. Because the default changes, the
  release note marks the change and names it under `Changed` in a major release.
