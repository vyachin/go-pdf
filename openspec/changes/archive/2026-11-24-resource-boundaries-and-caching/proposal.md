# Proposal

## Why

Three follow-ups from the review, each small and independent:

- `BaseDir` is documented as a hard boundary for stylesheet paths, but the check is lexical: a symlink inside the base directory that points outside passes it and the file is read. The boundary must hold for the filesystem, not just for the string.
- Every page lists every image of the document in its resources, even images used on other pages, and a stylesheet is read twice per conversion (validation and rendering). Both waste memory and output size.
- The kerning adjustment formula is correct but non-obvious; without an example in place the sign rule ("positive tightens the pair") is only discoverable from the PDF specification.

## What Changes

- Stylesheet loading SHALL be confined to the base directory through `os.OpenRoot`, so symlinks cannot escape it. Lexical rejections stay as they are.
- A page SHALL list only the images that page draws.
- One conversion SHALL collect stylesheets once and reuse them for validation and rendering; images SHALL be cached per source for the duration of the conversion, so a repeated image is embedded once.
- A comment with a concrete `TJ` example SHALL document the adjustment sign.

Out of scope: the remaining review findings (tables, inheritance, media queries, cancellation budgets, public API, importer removal).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the stylesheet path boundary requirement is extended to symlinks.
- `pdf-output`: adds per-page image resources and single embedding of a repeated image.

## Impact

- **Code**: `internal/profile/styles.go` (confined open), `internal/profile/profile.go` (validate using already collected sheets), `internal/engine/convert.go` and `internal/engine/images.go` (single collection, image cache), `internal/pdf/writer.go` (per-page resources, comment).
- **Output**: PDFs with images used on a single page become smaller because other pages stop listing them.
- **Behaviour**: a stylesheet reached through an escaping symlink is now rejected with a diagnostic instead of being read.
