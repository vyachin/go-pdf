# Design

## Context

See proposal.md. Relevant current behaviour:

- `resolveStylesheetPath` validates with `filepath.Abs`/`filepath.Rel` and then `os.ReadFile`; symlinks are resolved by the kernel at open time, bypassing the lexical check.
- `writer.Bytes` writes `/XObject` resources from the whole `images` slice for every page.
- `engine.Convert` calls `profile.Validate` (which collects stylesheets) and then `profile.CollectStylesheets` again; `imageLoader.Load` reads and decodes a file on every `<img>` occurrence.

## Goals / Non-Goals

**Goals:**

- A filesystem-level boundary that also survives symlinks and time-of-check/time-of-use races.
- Page resources that reflect what the page draws.
- One read of each stylesheet and image per conversion.

**Non-Goals:**

- Resource limits, cancellation, or a general asset cache across conversions.
- Changing the public API; all changes stay inside `internal`.

## Decisions

### 1. `os.OpenRoot` instead of `EvalSymlinks`

When `BaseDir` is set, the loader opens the base directory once with `os.OpenRoot` and opens the stylesheet through the returned root. `os.Root` resolves symlinks and refuses paths that leave the root, which is exactly the guarantee the API makes. The lexical check stays for error messages and for the no-base-directory case.

*Why not `EvalSymlinks` before reading*: it checks and then reads separately; a link can be swapped in between. `os.Root.Open` performs the check during the open.

*Why keep the lexical check*: it produces precise diagnostics (`css/link-path`) for `..` and absolute paths outside the base, and it keeps behaviour identical when `BaseDir` is empty.

### 2. Per-page image resources from the page's own operations

Before writing a page dictionary, collect the set of images that page draws by scanning its operations, then emit resource entries from the ordered global image slice filtered by that set. The order stays document order, so output remains deterministic.

*Why not keep the full list*: it makes every page depend on every image object, inflates page dictionaries and forces the writer to embed images referenced only on other pages.

### 3. One stylesheet collection, one image decode

`profile.Validate` is split into collection plus validation of collected sheets; `engine.Convert` collects once and validates the same slice it later feeds to the cascade, the page style and the font registry. `imageLoader` keeps a map from source path to decoded image, so a repeated `<img>` shares one `ImageData` and the writer's pointer-based deduplication embeds it once.

*Why a per-conversion cache and not a global one*: the loader is created per conversion; a global cache would keep file contents alive and risk stale data when templates change on disk.

### 4. Comment with an example

`advanceAdjustment` gains a comment showing `[<0041> 42 <0056>] TJ` and stating that a positive number moves the next glyph left, that is, tightens the pair.

## Risks / Trade-offs

- **`os.OpenRoot` availability**: it is part of the standard library since Go 1.24 and the module targets 1.24. Verified against the local toolchain.
- **Symlink inside the base that points outside** is now rejected, which may surprise a deployment that used such links intentionally. → The diagnostic names the path; the fix is to place the file inside the base.
- **Per-page filtering changes the PDF bytes** for every document with images. → Determinism tests compare runs against each other, not against old bytes; goldens are text-based.
- **Caching hides later file changes within one conversion** — acceptable: a conversion is a snapshot of inputs.
