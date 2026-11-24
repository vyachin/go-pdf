# Proposal

## Why

The base directory is documented as a boundary for document inputs, but only stylesheets are confined to it. Images and fonts are still read through `filepath.Join` plus `os.ReadFile`:

- an `<img src>` that is an absolute path bypasses the base directory entirely;
- a relative image path may climb out with `..`;
- a `@font-face` path resolves against `FontDir` or `BaseDir` and then reads whatever the path points at, including through a symlink.

The CSS loader already solves this with a confined open. The same rule must hold for every local asset the document names, otherwise the boundary is advertisement rather than enforcement.

## What Changes

- A single shared reader SHALL open document assets inside a configured root: it resolves symlinks and opens through `os.OpenRoot`, so a link swapped in after the check still cannot escape.
- Images SHALL be opened inside `BaseDir`; absolute paths outside it MUST be rejected with a diagnostic.
- Font files named by `@font-face` SHALL be opened inside the font root (`FontDir`, else `BaseDir`). The fallback font and system fonts are operator-provided trusted paths and stay outside the boundary, as documented.
- When no root is configured, behaviour stays as before.
- The header repetition in table layout SHALL move to its own method, so `placeTable` no longer carries a flag between phases. No behaviour change.

Out of scope: budgets and cancellation (ENG-02), and table/cascade fixes from the review.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the path boundary requirement is extended from stylesheets to every local asset named by the document.

## Impact

- **Code**: `internal/profile/styles.go` (shared confined reader), `internal/engine/images.go`, `internal/engine/fonts.go`, `internal/layout/table.go`.
- **Behaviour**: documents that referenced images or fonts outside the base directory now fail with an explicit diagnostic instead of being read. This is the intended correction; deployments must place assets inside the configured roots.
- **Tests**: symlink escape and absolute escape for images and fonts; an internal symlink still works; existing CSS path tests keep their codes; the two-row `thead` test guards the refactor.
