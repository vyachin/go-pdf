# Proposal

## Why

A text node is matched to its source token by comparing the token's raw bytes with
the tree's decoded text. A character reference never matches: `&amp;` is compared
with `&`, so every such node falls through to a full scan of all tokens. On a
document of `<p>&amp;</p>` fragments that scan is quadratic — 16,000 fragments take
about 3.7 s, 32,000 about 14 s — while the work guard records only one unit per
token and stays at 0.5% of its 20,000,000-unit default, so no `resource/limit` is
ever reported.

The same mismatch breaks the source-position contract. A text node with a
character reference does not match its own token and inherits its parent element's
range, so `text/font`, `text/glyph` and every other diagnostic raised for that text
point at the element instead of the text. In a document that also contains a
literal `&`, the fallback can even return a different node's span.

The existing requirements already forbid both outcomes: every element and text
node must carry the byte range it occupies, and a node's span must be the span of
its own token. The fix is a contract fix, not only a performance fix.

## What Changes

- Decode a text token once, in the token pass, where the tokenizer knows whether
  the token is raw text (`<style>`, `<script>`), RCDATA or normal text. Store the
  decoded text only when it differs from the raw bytes, interning equal strings.
- Match a text node against the decoded text of its token, so `&amp;`, `&rarr;`,
  `&#147;` and a legacy `&nbsp` keep their own byte range and their own position.
- Replace the per-node full scan of all tokens with a lazily built index of the
  decoded token text, used only when the tree places a node out of source order
  (foster parenting). The index is built once per document.
- Charge the position-matching work to the shared budget: one unit per text span
  indexed and one per out-of-order lookup, with the guard's cancellation checks.
  A document whose position matching exceeds the budget fails with
  `resource/limit` instead of doing uncharged quadratic work.
- Keep the existing position semantics for a token that matches no span: the node
  falls back to its parent's range. A token the parser trims (the leading newline
  of `<pre>`/`<textarea>`, a removed NUL) is not part of this change and is
  recorded as a known gap.
- Add a regression test for the entity span, a test that the matching is charged,
  an adversarial fixture registered in the corpus, and a benchmark that reports
  the scaling of the fragment document. Update the work-budget documentation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `html-parsing`: "A node's source span comes from its own token (RV4-10)" now
  defines matching after character-reference and line-ending decoding, so a text
  token with a reference keeps its own range. A new requirement bounds and
  charges the position matching: the decoded token text is indexed, the search is
  not repeated per node, the work is charged, and exhaustion is `resource/limit`.

## Impact

- `internal/html/scan.go`: record the decoded text of a text token when it differs
  from the raw bytes; no change to the recorded ranges.
- `internal/html/parse.go`: decoded fast path, lazy text index, guard threading
  through the builder, removal of the uncharged full scan.
- `internal/html`: new regression and cost tests; an adversarial fixture under
  `testdata/adversarial/` registered in `internal/engine/adversarial_test.go`.
- Diagnostics raised for text nodes may now point at the text instead of its
  parent element; this is the intended correction and changes positions, not
  codes. Near-limit documents that contain fostered text may now report
  `resource/limit` because the matching is charged.
- `docs/profile.md`: the HTML parsing row of the work-budget table.
- No public API change.
