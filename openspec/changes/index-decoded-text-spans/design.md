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

Two constraints shape the approach. First, only the tokenizer knows whether a text
token is raw text (`<style>`, `<script>`), RCDATA (`<title>`, `<textarea>`) or
normal text; decoding `&amp;` in raw text would be wrong. Second, the budget is a
shared `*budget.Guard` that already reaches the parser through `ParseOptions` but
is not threaded into the conversion.

## Goals / Non-Goals

**Goals:**

- A text node with a character reference keeps its own token's range and position.
- Matching work is linear in the document size: fast path for source-order tokens,
  one index for out-of-source-order tokens, built at most once per document.
- All matching work is charged to the shared guard and observes cancellation, so a
  hostile document stops with `resource/limit` instead of running quadratic work.
- No change to the recorded span partition or to the existing foster-parenting
  behavior.

**Non-Goals:**

- Fixing the range of a text node whose token the tree construction trims (the
  leading newline of `<pre>`/`<textarea>`, a removed NUL). Such a node keeps its
  parent's range, as today; this is recorded as a known gap.
- Changing the first-occurrence semantics for equal out-of-order texts. Two
  foster-parented texts with the same content keep pointing at the first
  occurrence, as today.
- Changing the public API or the output of a document whose positions already
  match.

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
decoding, line-ending conversion, NUL replacement). A token without them is
compared as its raw slice, so the common path pays no decode.

Decoded texts are interned (`map[string]int32` over the token pass's lifetime) so
repeated references such as `&amp;` or `&nbsp;` share one string. The id is stored
in the span's `name` field, which is unused for text spans; the alternative, a new
`int32` field, grows every span from 16 to 20 bytes, up to 32 MiB at the 8 MiB input
limit with many spans, for no benefit. The field is documented as a per-kind field.

### Decoded fast path for the source-order case

`textSpanFor` compares the candidate span's decoded text with `Node.Data` using
exact string equality. Character references, legacy references and CRLF now match
their own token, so the common path is unchanged in shape: skip non-text spans and
spans before `lower`, compare the next text span, advance the cursor on a match.
There is no budget charge here because this is the node's own token, already charged
by the token pass.

On a mismatch the cursor does not advance: the span may belong to a later tree node
whose source is ahead of the current one. This preserves the existing invariant.

### One lazy index for out-of-source-order lookups

When the fast path misses, the converter consults `map[string]int32` from decoded
text to the first span with that text, built once over all text spans and reused.
A miss in the index returns "no span", which is today's parent-range fallback.

Alternatives considered:

- **Eager index in the token pass.** Every document pays string allocation and a
  map insert per text token, including documents that never foster content. The
  lazy build pays only when the tree order actually diverges.
- **Keep the full scan and only charge it.** This bounds a hostile document at the
  budget, but it still performs up to 20,000,000 comparisons and makes a legitimate
  document with many fostered texts fail; the index makes the same document linear.
- **Hash-keyed index.** Smaller keys, but collisions need a verification and
  fallback path; a string key is exact and deterministic. If an implementation ever
  switches to hashes, collision resolution must charge every span it examines, as
  the spec requires.

The index stores the first occurrence only, which matches today's full-scan result
and keeps the map small for repeated text. The index is built in span order, so the
result is deterministic and independent of map iteration.

The fast path is tried before the index, so a document with one fostered text and
otherwise monotonic text builds the index once and then continues on the fast path.

### Charge the matching work through the builder

`builder` gains the guard and a sticky failure, and `ParseWithOptions` returns that
failure. The index build charges one unit per text span; an out-of-order lookup
charges one unit. The walk checks the failure at each node and stops, so a partially
converted model is discarded rather than completed. `budget.Guard.Add` performs the
context check every 512 calls, which makes cancellation prompt across a large index
build without a check per comparison. Returning errors from every builder method was
rejected as a wider, noisier signature change for the same behavior.

Because the charge is new, a document whose honest matching cost crosses its
`MaxOperations` now reports `resource/limit` where it previously converted. That is
the intended contract, and the default limit is 20,000,000 units.

## Risks / Trade-offs

- **A near-limit document with fostered text now stops** -> The work was real and
  uncharged before. Document the changed accounting in `docs/profile.md` and the
  changelog; the default budget leaves headroom.
- **Diagnostic positions for text with references change** -> Intended correction;
  affects positions, not codes. No golden or corpus expectation currently pins the
  old parent position, and the new regression test pins the correct one.
- **`z.Text()` mutates the tokenizer's buffer for the current token** -> The span
  records offsets into the immutable decoded document, and the raw length is
  captured before the call. The partition test and the raw-text style test guard
  this; add CRLF and reference cases to the position tests.
- **The lazy index keeps decoded text alive for the rest of the parse** -> Bounded
  by the input size plus one map entry per distinct text; freed with the scan
  result when the parse returns. Repeated references are interned, so the
  fragment document holds one `&` string, not 16,000.
- **Equal out-of-order texts still share one span** -> Out of scope; record it as a
  known limitation alongside the trimmed-token gap.
- **An implementation that scans instead of using the index** -> The budget charge
  makes a charged scan visible as `resource/limit`; the benchmark reports the
  scaling. A reverted scan that also drops the charge is caught by the entity
  correctness test only if the scan still mismatches, which is the original defect.

## Migration Plan

No API migration. The change is internal to `internal/html`, the adversarial corpus
and the work-budget documentation. A release note states that diagnostics raised for
text with character references now point at the text, and that matching work is
charged like every other parser stage. Rollback is a revert of the change; recorded
positions and budget units return to the previous values.

## Open Questions

None.
