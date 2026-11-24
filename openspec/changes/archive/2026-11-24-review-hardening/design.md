# Design

## Context

See proposal.md for motivation. Relevant current code:

- `internal/pdf/writer.go` allocates four objects per font (`fe.ids`), writes Type0, CIDFontType2, then overwrites the same id with the FontDescriptor. `/DescendantFonts` therefore points at a descriptor.
- `internal/font/gpos.go` checks four bytes at the start of a class definition and then reads a range header that needs six.
- `internal/layout/paginate.go` marks the page non-empty before content is placed and relies on a 5000-iteration cap, which ends with a warning and an empty but "successful" document.

## Goals / Non-Goals

**Goals:**

- A PDF whose font references are structurally valid and verifiable by following them.
- Font parsing that cannot panic on truncated input.
- Pagination that either makes progress or fails loudly.

**Non-Goals:**

- Fixing the other review findings listed in the proposal.
- Changing the public API beyond the new error diagnostic.

## Decisions

### 1. Five explicit font objects

Each font gets five ids: `fontID`, `cidID`, `descID`, `fileID`, `toUniID`. The links are written once each:

```
Type0 (fontID) --/DescendantFonts--> CIDFontType2 (cidID)
CIDFontType2  --/FontDescriptor----> FontDescriptor (descID)
FontDescriptor--/FontFile2---------> stream (fileID)
Type0         --/ToUnicode---------> stream (toUniID)
```

*Why not reuse the existing four-id layout*: it cannot express the chain; any compression of ids ends with one object playing two roles, which is exactly the defect.

*Verification*: a test parses the produced PDF, resolves `/DescendantFonts`, and asserts the target has `/Subtype /CIDFontType2`, then follows `/FontDescriptor` and `/FontFile2`. Searching the bytes for `/ToUnicode` is not a test of the graph and is explicitly rejected by the review.

### 2. Bounds checking per table format

`parseClassDef` validates the header length for its format before any field read: format 1 needs offsets up to `off+6`, format 2 up to `off+4` plus `count*6`. The same pattern is applied to the coverage and pair-set readers it depends on.

*Policy for corrupt optional tables*: a malformed `GPOS` is ignored and the font loads without kerning. Kerning is an optional feature; failing the whole document because a font has a broken optional table would be worse than losing kerning, and the review asks for "error or safe ignore". Required tables (`head`, `hhea`, `maxp`, `loca`, `glyf`, `hmtx`) keep returning errors.

### 3. Progress check instead of an iteration cap

The page loop records two numbers before placing content: the number of drawing operations on the page and the vertical position. After a failed placement and a page break, if the new page received no operations and the flow position did not advance, the loop stops with `layout/no-progress`.

```
before := (len(ops), y)
done := placeBlock(...)
if !done && pageDidNotChange(before) { error }
```

*Why not keep only the 5000-iteration cap*: the cap hides the defect (empty pages, lost text) and burns CPU. The progress check detects the real condition on the first page. The cap stays as a second line of defence, now reported as an error.

*Consequence*: `Convert` returns the diagnostic and no PDF; the CLI exits non-zero and writes no file.

### 4. Regression tests from the review

Each confirmed reproduction becomes a permanent test: the font graph walk, the truncated `parseClassDef` input, and the pagination case with `line-height: 100pt` on a 100pt page. The fuzz corpus gains a seed for the truncated GPOS.

## Risks / Trade-offs

- **Readers that tolerated the broken graph** may now render text differently — this is the intended correction; the change is verified with an independent validator that resolves the graph.
- **Ignoring broken GPOS** silently loses kerning. → Keep it deliberate: the font still loads, and the condition is visible in a unit test that exercises the truncation; adding a diagnostic channel to the font package would be a larger change.
- **Progress detection false positives**: a page may legitimately receive no operations (empty spacer block) while the flow advances. The check requires both "no ops" and "no advance", and an empty page followed by progress is allowed.
- **The cap remains** for pathological cases; it now reports an error, so no document is silently truncated.
