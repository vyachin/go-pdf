# Design

## Context

`internal/html` builds its tree with the standard construction algorithm from
`x/net/html`; the engine never mutates that tree after parsing, so the
child-list helpers on `Node` have no production caller. `css.PropOrder` is used
for the documented property order and by a snapshot test, while `css.Table` is
the parse table: two lists of the same names that can diverge silently.
`Token.Text` is read by the selector and value parsers, so its comment is the
contract for those callers. `docs/profile.md` states that its tables are the
profile, but only the `display` row is compared with code today.

## Goals / Non-Goals

**Goals:**

- No child-list helper that nothing calls and no comment that describes a
  removed call path.
- One failing test when `PropOrder` and `Table` disagree, and a truthful
  `Token.Text` contract.
- A profile document whose element, attribute and inheritance tables are
  compared with the code, with the three known gaps closed.
- No behavioural change to any conversion.

**Non-Goals:**

- Generating `docs/profile.md` automatically from code. The document carries
  explanations the tables cannot, and a generated file would turn every prose
  edit into a generator change.
- Replacing the standard tree construction with engine-side foster parenting.
- Renaming `Token.Text` or changing any tokenizer behaviour.

## Decisions

**Delete the unused helpers instead of testing them.** `InsertBefore`,
`InsertAt`, `relinkFrom` and `ChildIndex` are exported methods on an internal
type, so `staticcheck`'s `unused` check does not flag them; keeping them for a
hypothetical caller would need a test that proves nothing about the engine.
`relinkFrom` is also O(children) per insert, so routing foster parenting through
it would make a pathological parse quadratic. Alternative rejected: keep the
helpers and add a test — it preserves dead maintenance surface and the stale
foster-parenting comment with it.

**Keep `PropOrder` explicit and test it as a set.** The order is part of the
documented output and of a snapshot, so deriving it from map keys would make the
documented order unstable. The test compares the two sets and names the missing
name. Alternative rejected: delete `PropOrder` and sort the keys where it is
used — alphabetical order is not the order the document explains.

**Fix the token comment, not the tokenizer.** Ident, hash and at-keyword text is
already decoded and the selector parser depends on that; the bug is that the
comment claims the opposite. The comment will list the decoded kinds and the
kinds that keep their spelling, and a small test will assert the comment still
names them, so a future edit cannot silently reverse the contract again.

**Compare the document tables with the code in a package test.** A test in
`internal/profile` reads `docs/profile.md`, extracts the element code block, the
attribute table and the `Inherited` column of the property table, and compares
them with the code's allow-lists and `css.Table`. Alternative rejected: a
generated document — see Non-Goals.

**Correct the three documented facts.** The `<style>` row is added because the
code accepts `media` on the element; the `media` wording becomes "matches a
whole comma-separated item, so `only print` is skipped"; the `line-height` row
gains `%`, which the parser already accepts and a style test already pins.

## Risks / Trade-offs

- **Markdown parsing is brittle** → the test reads only the three delimited
  blocks and fails with a row name and the file, so a formatting change is a
  visible test failure rather than silent drift.
- **A hidden caller of a removed helper** → the change runs `go build ./...`,
  `go vet ./...` and the full test suite; the helpers have no reference outside
  `node.go` today.
- **The `only print` statement becomes a compatibility promise** → it matches
  the current `MediaAppliesToPrint`; if the parser later accepts `only`, the
  documented rule and the test change together.

## Migration Plan

None: internal cleanup and documentation. The archive notes that no exported
identifier, diagnostic or output changed. `docs/profile.md` consumers see three
corrected facts, not a new profile.
