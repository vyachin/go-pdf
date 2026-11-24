# Design

## Context

See proposal.md. Current readers:

```
styles.css   -> readConfined(path, BaseDir)   -> os.OpenRoot      OK
<img src>    -> filepath.Join(BaseDir, src)   -> os.ReadFile      no boundary
@font-face   -> resolvePath(src)              -> os.ReadFile      no boundary
fallback     -> os.ReadFile(FallbackFont)     -> trusted
system fonts -> os.ReadFile(system path)      -> trusted
```

## Goals / Non-Goals

**Goals:**

- One implementation of "read a file inside a root", used by CSS, images and fonts.
- Absolute paths and symlinks cannot leave the configured root.
- Table header repetition becomes its own step.

**Non-Goals:**

- Result caching across conversions, budgets, cancellation.
- Trusting or validating operator configuration: `FontDir` and `FallbackFont` are assumed to be set by the operator.

## Decisions

### 1. `profile.ReadWithin(root, path)`

The helper resolves the root and the path with `EvalSymlinks`, rejects a path outside the root with a sentinel `ErrOutsideRoot`, then opens the resolved relative path through `os.OpenRoot`. Resolving first keeps *internal* symlinks working (including absolute links that point inside the root); `os.Root` then closes the window for a link swapped in between.

An empty root means no boundary and a direct read, which preserves the current behaviour for callers that configure nothing.

*Why not move it to a new package*: `profile` is already the place that decides which local inputs are allowed; one exported helper there avoids a package whose only content is one function, and `engine` already imports `profile`.

### 2. Policy per asset class

| Asset | Root | Outside the root |
|---|---|---|
| Stylesheet | `BaseDir` | `css/link-path` |
| Image | `BaseDir` | diagnostic `img/load` naming the path |
| Font from `@font-face` | `FontDir`, else `BaseDir` | diagnostic `font/unavailable` |
| Fallback font | none (trusted) | read as configured |
| System fallback fonts | none (trusted) | read as configured |

*Why the fallback stays trusted*: it is an operator-provided absolute path, not content named by the document. Confining it would break `-font` and `PDFTEST_FONT` on developer machines.

### 3. Header repetition as a method

`paintRepeatedHeaders(f, t) bool` paints the header rows when the table continues on a new page and reports whether it did. `placeTable` calls it once and uses the result; the flag no longer travels between phases.

*Why a bool and not two methods*: callers need exactly one fact — were headers painted on this page — and the row loop uses it to skip duplicates.

## Risks / Trade-offs

- **Path inside the root but reached through an absolute symlink** now works (resolved first), which is the status quo for CSS and is what users expect.
- **Diagnostics change** from "cannot read" to "escapes the base directory"; tests pin the new text for CSS and the new codes for images and fonts.
- **A deployment that kept images outside `BaseDir`** now fails loudly at conversion time; the message names the path.
- **Refactor risk** none observed: behaviour is unchanged and covered by the existing two-row header test plus the corpus.
