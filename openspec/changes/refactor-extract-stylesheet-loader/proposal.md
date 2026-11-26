# Proposal

## Why

`internal/profile` is documented as the allow-list validator, but it also opens
files, decodes legacy encodings, stamps `@charset` positions and resolves
stylesheet paths. It imports `internal/assets`, `internal/fsx`, `internal/asset`
and `path/filepath`, and it resolves one href twice: `resolveStylesheetPath`
produces a path that the function throws away, then `assets.Read` resolves the
same reference again inside the registry. Two resolvers can disagree, and a
validator that reads the disk cannot be tested, reused or reasoned about
without an asset tree. The layering diagram in `CONTRIBUTING.md` does not show
that dependency at all.

## What Changes

- Extract loading, decoding and `@charset` handling into a new
  `internal/stylesheet` package that takes the conversion's asset registry and
  returns parsed stylesheets plus diagnostics.
- Delete `profile.resolveStylesheetPath`; the asset registry is the single path
  resolver, and the loader maps its typed errors to the existing `css/link-*`
  codes.
- Keep `internal/profile` as validation only: elements, attributes, declarations
  and `@font-face` descriptors, with no filesystem or asset import.
- Add an import-graph test that fails when `profile` gains an asset dependency,
  and a diagnostic regression matrix that proves the public codes and positions
  are unchanged.

No public API, diagnostic code, ordering or output change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the internal package boundary becomes a checked invariant:
  validation does not read assets, and one resolver owns path resolution.

## Impact

- New package `internal/stylesheet`; `internal/profile/styles.go` splits between
  it and `profile.go`; `internal/engine/convert.go` calls the loader instead of
  `profile.CollectStylesheets`.
- Tests move with the code: stylesheet loading and charset tests to
  `internal/stylesheet`, allow-list tests stay in `internal/profile`.
- `CONTRIBUTING.md` layering section names the new package.
- No exported identifier, option, diagnostic code, message or file position
  changes; golden documents are byte-identical.
