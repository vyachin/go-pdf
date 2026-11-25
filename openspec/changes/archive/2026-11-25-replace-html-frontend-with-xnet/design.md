# Design

## Context

`internal/html` is a three-file frontend: a tokenizer with a hand-written entity
table (`token.go`, 391 lines), a tree builder with hand-written implicit-end-tag
and fostering heuristics (`parse.go`, 463 lines) and the document model
(`node.go`, 321 lines). The model, not the parser, is the package's public
surface: 17 packages under `internal/` use `*html.Node`, `Document.Pos`,
`Node.Path`, `Node.Attr` and `Node.Start`, and 70 call sites build diagnostics
from `Document.Pos`/`Position`. Document snapshots record those positions.

Constraints that shape the approach:

- `library-api` requires a limit breach to be reported before the expensive
  allocation, and `engine.Limits` counts parsed tokens in `MaxOperations`.
- `css-profile` requires every diagnostic to carry file, line, column and path.
- `internal/css.SheetOptions.Base` is a byte offset into the HTML source, so an
  inline `<style>` must keep its exact source range
  ([parse.go:167](../../../internal/css/parse.go), [styles.go:67](../../../internal/profile/styles.go)).
- `document-layout` promises scoped implicit end tags and collapsed white space;
  `147_table-snapshot` and `bigtable` exercise large tables.
- Measured on the 95 corpus fixtures and 4 project documents: 92 documents get
  implied `html`/`head`/`body`, 15 get an implied `tbody` (59 nodes), 16 contain
  white-space-only text directly in table structure, and only 2 have a tree whose
  element order diverges from the token order (`Issue532`, `list-style-type`).

See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**

- Standard character references and standard tree construction replace the
  hand-written subset, behind the existing `internal/html` API.
- Every element keeps a source position; authored elements keep exact
  coordinates, nodes created by the parser inherit an ancestor's.
- `MaxDOMDepth`, `MaxOperations` and cancellation keep their current meaning and
  ordering relative to tree allocation.
- Text and render snapshots stay byte-identical; document snapshots change only
  where the standard tree adds an implied element, and every such line is
  reviewed.

**Non-Goals:**

- Character-encoding detection (windows-1251, `golang.org/x/net/html/charset`).
  The input contract stays "UTF-8 bytes"; this is a separate change.
- Layout or profile behaviour for `svg`/`math` content: it stays unsupported and
  rejected exactly as today.
- Fragment parsing, `html.Render` as an output path, or exposing `x/net` types
  outside `internal/html`.
- Changing diagnostic codes or their wording.

## Decisions

### D1. Adopt the standard tree, not only the tokenizer

`html.Parse` builds the tree; the in-house tree builder is deleted. Alternative:
keep our builder and swap only the tokenizer. Rejected because the tokenizer is
the smaller liability — the measured divergence between our builder and the
standard (implied `tbody`, fostering, RCDATA, adoption agency) is exactly the
class of bug a customer template finds, and the token order already matches the
tree order for 97 of 99 measured documents, so keeping the builder buys no
position fidelity that alignment does not already provide.

### D2. Convert into the existing `Node` model

The `x/net` tree is converted into `internal/html.Node` rather than replacing the
model. Alternative: keep `*html.Node` and hold positions in a side table.
Rejected: it would rewrite 17 packages, `Path`, `Attr`, `ID`, `Class`, `Document`
helpers and every `doc.Pos` call, and the side table would still be the same
alignment problem.

### D3. One token pass before the tree

`ParseWithOptions` runs:

```
src
 |-- [1] scan      : tokenizer over src
 |                    budget.Add(1) per token, depth check, cancellation,
 |                    compact span list: {kind uint8, name uint16, start,end int32}
 |-- [2] parse     : html.Parse(bytes.NewReader(src))
 |-- [3] convert   : *html.Node -> *Node, positions from [1] in document order
 `-- [4] normalize : text merge, table white space, attribute mapping
```

Alternative: parse first and enforce limits on the resulting tree. Rejected: an
8 MB document of `<div>` start tags allocates about 1.6 M nodes before the depth
check runs, which contradicts the `library-api` scenario "before the expensive
allocation". Alternative: only a counting reader for the budget. Rejected: it
cannot enforce depth and provides no offsets.

Cost: the input is tokenized twice. For the 8 MB `MaxInputBytes` ceiling that is
tens of milliseconds; `bigtable` (12 KB) measures below noise.

Memory: spans are 12–16 bytes each, so the 8 MB worst case (about 2 M tokens)
holds roughly 32 MB. Typical business templates stay under 1 MB. This is
accepted instead of a third pass, which would triple lexing and still need a
token cursor.

### D4. Positions by greedy alignment in document order

For each element in the converted tree, take the next unconsumed start tag with
the same name; text nodes take the span of the text run they were merged from.
An element with no matching token (implied `html`/`head`/`body`/`tbody`, cloned
formatting elements) inherits the range of its nearest positioned ancestor, and
its `Path()` disambiguates it. The two documents where the standard tree
reorders content (`Issue532`, `list-style-type`) degrade to inherited positions
for the reordered subtree only; no position is ever negative or zero, and no
diagnostic claims a line that does not exist.

### D5. Normalizations, all in one pass, all specced

- Adjacent text nodes are merged, because layout assumes one node per run.
- White-space-only text directly inside table structure is dropped, preserving
  today's layout for the 16 measured documents; the spec states the observable
  rule ("takes no part in layout") rather than where the drop happens.
- `x/net` attributes carry a namespace; it is flattened to `prefix:name`
  (`xlink:href`). Namespace-less attributes keep the lower-cased key.
- `/>` on a non-void element is ignored, as the standard says; today we honour
  it. This is the one deliberate behaviour change besides the tree itself.
- `<style>` text is re-sliced from the document source using its span, not taken
  from the tokenizer's unescaped, newline-normalized text, so `SheetOptions.Base`
  arithmetic stays exact for CRLF input.

### D6. Limits keep their units

`MaxOperations` keeps counting one unit per token in [1], so existing budgets and
tests remain comparable. Depth is tracked in [1] with the standard void element
set; a document over `MaxDOMDepth` is rejected with `resource/limit` before [2]
runs.

### D7. Dependency: `golang.org/x/net` v0.62.0, exact pin

The package imports only the standard library and `golang.org/x/net/html/atom`,
so the build graph grows by one module. Its own `go 1.26.0` directive matches
ours, so the `go` directive does not move. `go.sum` is committed; the existing
Dependabot `gomod` entry already covers updates.

### D8. An incomplete tag at the end of the input fails the parse

The standard tree construction drops a tag that is still open when the input
ends, so `<img src="` yields an empty document and, without a check, an empty
PDF with no diagnostic. The token pass already knows the offset of that leftover
input, so the parse returns a typed error and the engine reports it as
`html/truncated` at that offset. Alternative: convert the shorter document and
warn. Rejected because the output would silently omit content the author wrote,
which is the failure mode this engine exists to prevent.

## Risks / Trade-offs

- [Document snapshots gain implied `tbody` lines in 15 of 99 documents] →
  regenerate with the update flag, review each diff, and fail the task if a diff
  contains anything but implied elements.
- [White space inside tables now exists in the standard tree] → dropped in
  normalization; `147_table-snapshot`, `bigtable` and the 16 measured documents
  are the regression set.
- [Two fixtures reorder content] → positions inherited from ancestors there;
  snapshots reviewed by hand, and no test asserts a position from the reordered
  subtree without the inherited-position rule.
- [The corpus classification shifts] → re-measure `cases.json`; a fixture may
  move between `issue` and `snapshot` only together with an explanation in the
  test file, never by editing goldens blindly.
- [Diagnostics that used a `<style>` element's `Start` now depend on a span] →
  covered by a CRLF fixture that asserts the reported CSS line.
- [A regression in `document-layout`'s scoped implicit end tags] → the nested
  table and nested list scenarios already exist as tests; they are the acceptance
  gate for D1.
- [Double lexing costs CPU on large documents] → benchmark `bigtable` and a
  generated 1 MB table before and after; a regression above the noise floor
  reopens D3.

## Measurements

Measured on this machine (Go 1.27.4, Apple silicon), both before and after:

| Case | Before | After |
|---|---|---|
| `bigtable` (12 KB) | 8.4–11.4 ms | 9.5–11.9 ms |
| generated 1 MB table | 838–841 ms | 855–856 ms |

The conversion is dominated by layout, so the second tokenization pass stays
inside run-to-run noise; a 1 MB document costs about 2 % more wall time and
allocates slightly less (1.18 GB against 1.24 GB over three runs).

Corpus after the change: 38 convert, 57 rejected, 54 skipped — the same
conversion set as before. Six fixtures that used to fail with `img/load` and one
with `html/unsupported-element` now fail with `html/truncated`, which names the
real defect: their extracted HTML ends inside a tag.

## Migration Plan

The replacement happens behind `internal/html`, so it lands as one change with
these steps: dependency and policy first (so `go.mod` is honest), then the new
scan/convert/normalize frontend with its own tests, then the engine switch, then
snapshot regeneration and corpus re-measurement, then docs. Rollback is a revert
of the change: `internal/html` is self-contained and the model, `ParseOptions`
and `Document` API do not change, so no caller needs a compatibility shim.

## Open Questions

None. Charset detection is deliberately deferred to a separate change, and it
does not affect the specs or the task breakdown here.
