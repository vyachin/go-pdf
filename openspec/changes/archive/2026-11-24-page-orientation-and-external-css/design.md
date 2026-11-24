# Design

## Context

Currently the page size is parsed in `css.ParsePageSize` as a name (`A4`, `A5`, `Letter`) or two lengths; the fallback size from the launch parameters is applied later in `layout.ParsePageStyle`, which knows only the whole `size`. Styles are collected by `profile.ExtractStyleSheets`: a DOM walk, text nodes of `<style>`, offsets are shifted by the node position, and a diagnostic computes the position via `doc.Position(offset)` — that is, always inside the HTML. `<link>` and `<script>` are absent from the element allow-list and are rejected. There is no strict mode flag.

See `proposal.md` — motivation and scope.

## Goals / Non-Goals

**Goals:**

- Page orientation as part of the `size` grammar, consistent with the launch fallback size.
- Local stylesheets in document order, with diagnostics that point into the included file.
- A predictable script policy: do not execute, do not render, report; in strict mode — refuse.
- A fixture that verifies pagination of a long table.

**Non-Goals:**

- Network: remote styles and fonts remain forbidden.
- `@import` inside CSS.
- Media queries other than `print` and `all`.
- JavaScript execution and interactivity.
- An orientation flag in the CLI (`-landscape`): orientation is set by the document.

## Decisions

### 1. Parsing `size` separately from resolving the orientation

`css.ParsePageSize` returns not a ready size but a parsed description:

```go
type PageSizeSpec struct {
    Base        PageSize // zero when no size is named
    HasBase     bool
    Orientation Orientation // Any, Portrait, Landscape
}
```

`layout.ResolvePageSize(spec, fallback)` applies the orientation: with `landscape` the sides are swapped, with `HasBase == false` the `fallback` from the launch parameters is taken.

*Why not pass the fallback into the parser*: then `css` would start depending on launch settings, and the profile validator on them too, although the check only needs the grammar. The separation keeps `css` a pure value layer.

*Alternative*: resolve in `engine` — but then `@page` stops being self-sufficient for `layout`, which builds margin boxes from an already resolved size.

### 2. Rule source instead of an offset in the HTML

Every `css.Stylesheet` gets its own source (`File`, `Src`, line index) and can compute a position from an offset. `Rule` and `Declaration` reference their stylesheet, and `profile` converts this into a `diag.Position`.

```
   <style> in HTML --> Stylesheet{File: "", Locate: doc.Position}
   <link> from a file --> Stylesheet{File: "styles/print.css", Locate: local index}
                            |
                            v
                 rule.Sheet.Position(rule.Offset)
```

*Why not compute `diag.Position` directly in the parser*: `css` would start importing the diagnostics package and knowing about the HTML document. A reference to the sheet keeps the layers in the current dependency order.

*Cost*: rules gain a reference field, and all places that previously called `doc.Position(offset)` move to `rule.Sheet.Position`. This is a mechanical but broad edit of the position tests.

### 3. One ordered walk instead of two sources

`profile.CollectStylesheets(doc, opts)` walks the DOM once and returns `[]*css.Stylesheet` in order of appearance: `<style>` → parsed in place, `<link rel="stylesheet">` → file read and parsed. The engine passes the slice to `css.NewCascade` variadically, so the cascade order matches the document without additional sorting.

*Why not merge into one sheet*: merging loses the file boundary, and that boundary is exactly what is needed for positions. Besides, the slice allows caching the parse of a file later.

### 4. Rules for `<link>`

- `rel` is case-insensitive, only `stylesheet`; other values (`icon`, `preload`, `canonical`) are ignored without diagnostics.
- `media`: empty, `print`, `all` — apply; anything else — skip the sheet entirely (not an error: this is normal practice for print templates).
- `href`: an absolute or relative path to a file. The `http`/`https` scheme — error `css/link-remote`, `data:` — error `css/link-unsupported`.
- If `BaseDir` is set, the resolved path SHALL remain inside it: check via `filepath.Rel` and refusal if `..` is at the beginning. An absolute path outside `BaseDir` is rejected.
- A missing file — error `css/link-load` with the path; the document is not built.

*Alternative*: allow the network with a host allow-list — ruled out: the engine fundamentally makes no network requests, this is part of its contract.

### 5. Script policy

`<script>` is not on the element allow-list and does not become a box: `script { display: none }` is added to the engine's stylesheet, and `profile` emits the warning `html/ignored-script` instead of an error when walking elements. The script content already does not get into the tree as markup — the parser returns it as raw text.

*Why not add `script` to the allow-list*: that would look like support, whereas the engine merely cuts the tag out.

### 6. Strict mode

`engine.Options.Strict bool` → `profile.Options{Strict: true}` → any warning becomes an error with the same code and `Severity: Error`. The CLI gets `-strict` on `validate` and `convert`.

*Why all warnings and not just scripts*: one switch is easier to explain than a list of "strict" codes; besides, `layout/overflow` should also fail the build in strict mode.

### 7. Large document fixture

`testdata/bigtable.html`: 35 rows by 11 columns, Russian text, a header in `thead`, totals in `tfoot`, `@page { size: A4 landscape }`. Written from scratch; documents from html-to-pdf-tests are not copied (the repository has no license), they are used only as a reference to a realistic shape.

## Risks / Trade-offs

- **Broad position edit**: the rule source affects the parser, cascade, validator and tests. → Do it as a separate step before `<link>`, verify the existing position tests after each step.
- **Silent loss of scripts**: a template with a JS chart will be built without the chart. → A warning in the report, `-strict` for CI; `docs/profile.md` states explicitly that JS is not executed.
- **Bypassing `BaseDir`**: with an empty `BaseDir` the confinement does not apply, and a relative path resolves from the working directory. → Document: the service must set `BaseDir`.
- **Orientation without a size name**: the result depends on `-page`. → An explicit scenario in the spec and a test: `size: landscape` with `-page A5` yields 210×148 mm.
- **Stylesheet order**: a bug in the DOM walk will give a wrong cascade. → Test: `<style>`, then `<link>`, then `<style>` — the last one wins.
- **Strict mode changes the exit code** for templates with warnings. → Only with an explicit `-strict`; the default behavior does not change.
