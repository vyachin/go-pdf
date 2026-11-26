# Proposal

## Why

Source positions are computed by rescanning from the start of the line on every
call, and an element path is built by repeatedly appending to a string. A minified
one-line document therefore makes diagnostics and layout provenance quadratic:
8,000 `Document.Position` calls on a 112 KB line take 142 ms, and 8,000 validation
warnings on one line take 231 ms against 12 ms with newlines. One element with
40,000 class tokens takes 1.14 s to build its path because every append copies the
accumulated string, while the guard charges one unit per class. Separately, class
tokens are split with `strings.Fields`, which treats U+00A0 as a separator, so
`class="a\u00a0b"` matches `.a` although HTML splits class tokens on ASCII
whitespace only.

## What Changes

- Make source-position lookup incremental per line (the same cursor the charset
  mapper already keeps), so a position costs the bytes since the previous query on
  that line, not from the line start; charge the scanned bytes to the work budget.
- Build `Node.Path` with a `strings.Builder` and charge per byte written, so the
  cost is linear in the path length.
- Split class tokens on ASCII whitespace (`' '`, `\t`, `\n`, `\f`, `\r`) only, and
  cache the token slice on the node, whose attributes are immutable after parsing.
- Add the UTF-8 no-BOM case, a one-line document, and a no-break-space class to the
  parser tests and benchmarks.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `html-parsing`: position and path lookup gain a linear-cost rule and a charging
  rule; class tokenization gains the ASCII-whitespace rule.

## Impact

- `internal/html/node.go` (`Position`, `Path`, `Class`, `HasClass`), its callers in
  `internal/profile` and `internal/layout`; no diagnostic text or public API change.
- Tests in `internal/html`, `internal/profile`; benchmarks for position lookup and
  path building.
- No output change for documents with newlines and without no-break spaces.
