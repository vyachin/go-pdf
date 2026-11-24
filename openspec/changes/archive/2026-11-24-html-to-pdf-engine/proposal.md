# Proposal

## Why

An HTML → PDF converter in Go is needed for printed business documents (invoices, acts, contracts, reports), one that ships as a single static binary — no cgo and no external processes.

Off-the-shelf options do not meet this requirement. Chromium (`chromedp`, `go-rod`) and WeasyPrint are external processes with a heavy runtime. The pure-Go port of WeasyPrint (`benoitkugler/webrender` + `go-weasyprint`) is the only serious in-process candidate, but it is alpha, with one maintainer and unpredictable CSS coverage. For documents whose layout must reproduce for years, depending on such a core is a bigger risk than the cost of our own engine.

The key consideration: we do not need a browser. We need a deterministic document layout engine. That lets us drop 80% of CSS and gain in return what off-the-shelf engines do not give: guaranteed pagination, byte-for-byte reproducibility and predictable behaviour at the edges.

## What Changes

- A new Go module appears — an HTML-to-PDF rendering engine built with `CGO_ENABLED=0` that starts no child processes.
- A pipeline of six layers: HTML tokenizer and tree → CSS parser, cascade and selectors → box tree → layout and pagination → text shaping → PDF writing.
- **The CSS profile is fixed as a contract**: an allow-list of elements, properties, units and at-rules. Everything outside the profile is a template validation error, not a silent ignore.
- **Pagination is designed from day one**: `break-*`, `orphans`/`widows`, repeated `thead`, `@page` with margin boxes and `counter(page)` / `counter(pages)` counters.
- **Deterministic output**: fixed `/ID` and `/CreationDate`, the same input gives a byte-identical PDF.
- **A correct text layer**: TrueType subset embedding, `Identity-H` + `ToUnicode`, so Russian text can be copied and searched.
- A CLI and a Go library with a shared core are added; the profile linter is available as a separate check for template CI.
- Explicitly **out of scope for v1**: `flex`, `grid`, `float`, `position: absolute/fixed` (except fixed page headers and footers), gradients, shadows, transparency, rounded corners, `hyphens: auto`, bidi, CFF/OTF and variable fonts, web fonts by URL, JavaScript, link annotations.

## Capabilities

### New Capabilities

- `css-profile`: The allow-list of supported HTML and CSS, the cascade and inheritance rules, diagnostics for the unsupported — what makes the input contract checkable before rendering.
- `text-shaping`: Font loading, TrueType parsing, mapping characters to glyphs, kerning, line breaking and hyphenation rules.
- `document-layout`: Box tree construction, block and inline layout, fixed-layout tables, lists.
- `pagination`: Page breaking, `@page` and margin boxes, page headers and footers, page counters and two-pass `target-counter` resolution for the table of contents.
- `pdf-output`: PDF serialization, font subset embedding and `ToUnicode`, images, metadata, bookmarks and output determinism.

### Modified Capabilities

None: the project is new, there are no existing specs (`openspec/specs/`).

## Impact

- **Code**: a new Go module in an empty repository; there was no code before. Runtime dependencies: the Go standard library only.
- **Test oracles, not runtime**: `pdfcpu validate` and veraPDF are wired into tests to check output validity; they do not go into the binary.
- **Estimate**: ~60–80 person-weeks, 14–18 months for one engineer at full load. Milestone M3 (tables + pagination + headers and footers) in month 5–6 covers invoices and acts — that is the first useful release.
- **Risks**: a single engineer (bus factor 1); the amount of CSS we will want to support will grow, so the profile must be defended by the linter and review; the quality of hyphenation and pagination edge cases needs long seasoning on real documents.
- **External systems**: not affected. Consumers are services that generate documents and CLI users.
