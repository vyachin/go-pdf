# Design

## Context

See [proposal.md](proposal.md) for the motivation. The parser runs one token pass
(`internal/html/scan.go`) that records a byte range per token, then lets
`golang.org/x/net/html` build the tree, then converts the tree into this package's
model (`internal/html/parse.go`). The token pass charges one work unit per token;
the conversion charges nothing. A text node is matched to a token by comparing the
tree's `Node.Data` (decoded, e.g. `&`) with the token's raw bytes (e.g. `&amp;`),
which never matches a character reference. The conversion then falls back to a full
scan of every span, per node, with no budget check and no cancellation.

Three constraints shape the approach. Only the tokenizer knows whether a text
token is raw text (`<style>`, `<script>`), RCDATA (`<title>`, `<textarea>`) or
normal text; decoding `&amp;` in raw text would be wrong. The tree construction can
change a token's text after decoding: it drops one leading newline in
`pre`/`listing`/`textarea`, removes a NUL in body and table content, and replaces
it with U+FFFD in foreign content. Finally, the budget is a shared
`*budget.Guard` that reaches the parser through `ParseOptions` but is not threaded
into the conversion.

## Goals / Non-Goals

**Goals:**

- A text node with a character reference keeps its own token's range and position.
- A text node whose token the tree trims keeps a range that starts at its own
  text: a dropped leading newline is skipped, a NUL keeps the token's range.
- Equal texts matched out of source order never share a span.
- Matching work is linear: a fast path for source-order tokens and one index,
  built at most once per document, for out-of-source-order tokens.
- All matching work is charged to the shared guard and observes cancellation, so a
  hostile document stops with `resource/limit` instead of running quadratic work.
- No change to the recorded span partition or to the existing foster-parenting
  behavior.

**Non-Goals:**

- Changing the public API or the output of a document whose positions already
  match.
- Representing a non-contiguous source range: a NUL removed from the middle of a
  token keeps the whole token's range, which the specification allows.
- A hash-keyed index. If an implementation switches to hashes, collision
  resolution must charge every span it examines.
- Regenerating golden outputs: no current golden or corpus expectation pins the
  old parent position of an affected text node.

## Decisions

### Decode in the token pass, not in the converter

The scan calls `z.Text()` for a text token whose raw bytes contain `&`, `\r` or
NUL, and stores the decoded text when it differs from the raw bytes. The tokenizer
applies raw-text, RCDATA and normal-text rules itself, so `<style>p{content:"&amp;"}`
keeps `&amp;` while `<p>&amp;</p>` becomes `&`. Decoding in the converter with
`html.UnescapeString` was rejected: it has no way to know the token is raw text and
would corrupt inline CSS offsets.

The check for `&`, `\r` and NUL is exact rather than a heuristic: those are the only
inputs on which the tokenizer's decoded text differs from the raw bytes (entity
decoding, line-ending conversion, NUL replacement in raw text). A token without
them is compared as its raw slice, so the common path pays no decode.

Decoded texts are interned (`map[string]int32` over the token pass's lifetime) so
repeated references such as `&amp;` or `&nbsp;` share one string. The id is stored
in the span's `name` field, which is unused for text spans; the alternative, a new
`int32` field, grows every span from 16 to 20 bytes, up to 32 MiB at the 8 MiB input
limit with many spans, for no benefit. The field is documented as a per-kind field.

### Fast path: decoded text first, then the tree's variants

`textSpanFor` first compares the candidate span's decoded text with `Node.Data`
using exact string equality, after skipping non-text spans and spans before
`lower`. On a match it advances the cursor and records the span as used. This is
the node's own token, already charged by the token pass, so it costs no extra unit.

When exact equality fails, the fast path tries the forms the tree construction
produces for that token:

- **NUL.** If the decoded text contains a NUL, try it with each NUL replaced by
  U+FFFD and with each NUL removed. Either form accepts the token's whole range:
  the removed character is inside the token, and a single byte range cannot
  represent the remaining bytes as contiguous.
- **Dropped leading newline.** If the node's parent is `pre`, `listing` or
  `textarea` and the decoded text starts with `\n`, try `data == decoded[1:]` and
  start the range at the raw byte after the newline. The raw prefix is `\r\n` (two
  bytes), `\n` or `\r` (one byte), or a reference that decodes to a newline
  (bounded at 32 bytes; a numeric reference such as `&#10;` counts as its own
  bytes). The prefix is found by decoding only the token's first bytes, so the
  work is bounded.

Context variants stay in the fast path because the parent is known there. The
index below applies the same variants for out-of-source-order nodes.

### One lazy occurrence index, with context variants

On the first fast-path miss, the converter builds an index over all text spans and
reuses it. It holds every occurrence in document order, not just the first:

- `exact map[string][]hit` for decoded text, with `hit{span int32, start int32}`
  where `start` is an offset inside the span. A NUL-bearing span is also indexed
  under its replaced and removed forms, so a lookup by the tree's data finds it.
- `trim map[string][]hit` for decoded texts starting with `\n`, keyed by
  `decoded[1:]`, with `start` after the newline. It is consulted only for nodes
  whose parent is `pre`, `listing` or `textarea`, so a `div` text `\nX` cannot be
  matched to a pre token's trimmed key.

Alternatives considered:

- **First occurrence only.** Smallest index, but it hands the same span to the
  second node with equal text, which the specification now forbids.
- **Eager index in the token pass.** Every document pays string allocation and a
  map insert per text token, including documents that never foster content. The
  lazy build pays only when the tree order actually diverges.
- **Keep the full scan and only charge it.** This bounds a hostile document at the
  budget, but it still performs up to 20,000,000 comparisons and makes a legitimate
  document with many fostered texts fail; the index makes the same document linear.
- **Variant matching only in the fast path.** An out-of-source-order `pre` text or
  NUL text would still fall back to its parent's range, so the index needs the
  variants too.
- **Hash-keyed index.** Smaller keys, but collisions need a verification and
  fallback path; a string key is exact and deterministic.

### Consume occurrences in document order

Two nodes with equal text must not receive the same span, and the fast path may
already have consumed an earlier occurrence before the index exists. The builder
therefore keeps a `used` marker with one entry per span, allocated with the
builder. The fast path marks the span it matches; the index lookup advances a
per-key cursor past used hits, returns the first unused hit, and marks it. A span
indexed under several keys (exact, NUL variant, trimmed) is still one span, so
marking it used makes every key skip it.

The marker costs one byte per span — 8 MiB at the 8 MiB input limit with a span
per byte, against a span slice of roughly 128 MiB. A bitmap is an easy later
optimization. The per-key cursors keep the skip total linear in the number of
occurrences, so repeated equal texts cannot make a lookup quadratic.

Consuming in document order is also what makes the resolution deterministic: the
first out-of-source-order node with a given text takes the first unused occurrence.

### Charge the matching work through the builder

`builder` gains the guard and a sticky failure, and `ParseWithOptions` returns that
failure. The index build charges one unit per text span; each out-of-source-order
lookup charges one unit; a fallback scan, if one is ever used, charges one unit per
span examined. The walk checks the failure at each node and stops, so a partially
converted model is discarded rather than completed. `budget.Guard.Add` performs the
context check every 512 calls, which makes cancellation prompt across a large index
build without a check per comparison. Returning errors from every builder method
was rejected as a wider, noisier signature change for the same behavior.

Because the charge is new, a document whose honest matching cost crosses its
`MaxOperations` now reports `resource/limit` where it previously converted. That is
the intended contract, and the default limit is 20,000,000 units.

## Risks / Trade-offs

- **A near-limit document with fostered text now stops** -> The work was real and
  uncharged before. Document the changed accounting in `docs/profile.md` and the
  changelog; the default budget leaves headroom.
- **Diagnostic positions for text with references or trimmed tokens change** ->
  Intended correction; affects positions, not codes. No golden or corpus
  expectation currently pins the old parent position, and the regression tests pin
  the correct ones.
- **`z.Text()` mutates the tokenizer's buffer for the current token** -> The span
  records offsets into the immutable decoded document, and the raw length is
  captured before the call. The partition test and the raw-text style test guard
  this; add CRLF and reference cases to the position tests.
- **The index stores every occurrence and keeps decoded text alive** -> Built
  lazily, charged per span, and bounded by the input size plus one hit per
  occurrence; freed with the scan result when the parse returns.
- **The used marker costs one byte per span on every parse** -> It is proportional
  to the span slice and needed to keep equal out-of-source-order texts apart. A
  bitmap is a later optimization; the memory invariant has ample headroom.
- **Equal texts are ambiguous by nature** -> Document order defines the answer, as
  the new requirement states; it is deterministic and testable.
- **An implementation that scans instead of using the index** -> The budget charge
  makes a charged scan visible as `resource/limit`; the benchmark reports the
  scaling. A reverted scan that also drops the charge is caught by the entity
  correctness test only if the scan still mismatches, which is the original defect.

## Migration Plan

No API migration. The change is internal to `internal/html`, the adversarial corpus
and the work-budget documentation. A release note states that diagnostics raised for
text with character references, a trimmed leading newline or a NUL now point at the
text, and that matching work is charged like every other parser stage. Rollback is
a revert of the change; recorded positions and budget units return to the previous
values.

## Open Questions

None.
