# Proposal

## Why

The HTML frontend is the part of the engine we wrote and maintain ourselves, and
it is a deliberate subset: 45 named character references instead of the standard
set, no `RCDATA` for `title` and `textarea`, and hand-written heuristics instead
of the standard tree-construction algorithm. Every customer template that comes
from Word, Google Docs or a store export is a new test case for that subset,
while `golang.org/x/net/html` — maintained by the Go team, fuzzed, and checked
against the html5lib suite — already implements the standard.

It is also the part where a real defect hides: `&nbsp` without a semicolon,
`&rarr;`, or `&#147;` from a windows-1252 document reach the page as literal
text or disappear, and the current tests do not see it because the committed
corpus happens to use only the six entity names we already support.

The module policy currently forbids depending on it. That policy is stale: it
was written for a module with no requirements, and the module already has a
`go.sum` for `golang.org/x/text` that the test harness needs. Keeping the claim costs us
a maintained parser and buys nothing.

## What Changes

- Replace the in-house HTML tokenizer and tree builder with
  `golang.org/x/net/html` v0.62.0: `html.Parse` builds the standard tree, which
  is converted into the existing `internal/html.Node` model so that the rest of
  the engine keeps its current API.
- Keep source positions on every element: a token pass records byte offsets
  before parsing, and the tree is matched against that token stream in document
  order. Authored elements keep exact coordinates; nodes the parser creates
  itself (implicit `html`/`head`/`body`/`tbody`, clones from the adoption agency
  algorithm) inherit the position of their nearest positioned ancestor.
- Preserve the resource contract: the token pass still enforces `MaxDOMDepth`
  and the operation budget, and still reports context cancellation, before the
  tree is allocated.
- Normalize the standard tree to the print profile where the current behavior is
  intentional, and record every such normalization rather than letting it stay
  implicit.
- Report a document that ends inside a tag as `html/truncated` instead of
  converting the shorter document the standard tree construction produces.
- Remove the "module stays dependency-free" rule and replace it with an explicit
  allowlist: the standard library, or a module maintained by the Go team,
  pinned and checksummed in `go.sum`.
- **BREAKING** (documentation only): the README no longer claims the package has
  no dependencies, and `go.mod` gains a requirement.

## Capabilities

### New Capabilities

- `html-parsing`: how an HTML document becomes the profile document model —
  character references, tree construction, the normalizations applied to the
  standard tree, the source position every node carries, and the limits enforced
  before the tree exists.

### Modified Capabilities

- `code-quality`: the dependency-free module rule is replaced by a dependency
  allowlist that admits the standard library and Go-team-maintained modules.

## Impact

- `internal/html`: `token.go` and `parse.go` are replaced by a tokenizer
  adapter, a converter from `*html.Node`, and a normalizer; `Token`,
  `Node`, `Document` and `ParseOptions` stay, so the 17 importing packages change
  only where they touch position or attribute details.
- `internal/profile`, `internal/engine`, `internal/css`, `internal/layout`,
  `internal/docdump`, `internal/browser`: no API change expected; positions and
  attribute keys are the areas to verify.
- `go.mod`/`go.sum`: add `golang.org/x/net v0.62.0` (its own `go 1.26.0`
  directive matches ours).
- Golden artifacts: document snapshots gain implicit `tbody` elements for 15 of
  the 99 measured documents; text and render snapshots are expected to stay
  unchanged and any difference is a defect to explain, not to regenerate
  blindly.
- `testdata/mpdf/cases.json`: the accepted/rejected classification of the 149
  fixtures is re-measured; entries may move between `issue` and `snapshot`.
- Documentation: `README.md` (install and guarantees), `CHANGELOG.md`,
  `CONTRIBUTING.md` where it describes the parser.
