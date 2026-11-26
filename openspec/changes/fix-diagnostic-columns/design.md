# Design

## Context

Four position paths exist and each was written when its source kind was added:

- `html.Document.Position` counts runes from the line start for a UTF-8 document with no
  mapper, and counts bytes when a mapper is present.
- `charset.Mapper.Position` walks the source widths byte by byte and returns
  `srcCol + 1`.
- `css.Stylesheet.Position` counts bytes from the line start when no `Locate` function
  is installed.
- `charset.sourcePosition` counts bytes from the line start for a decoding error.

The documented rule is characters (`docs/profile.md`, README). The measured difference
is visible: 1:14 becomes 1:20 when a BOM is added, and a linked UTF-8 sheet reports 1:29
for character column 26.

## Goals / Non-Goals

**Goals:**

- One character-column rule and one implementation for all four paths.
- A BOM never shifts a column.
- Legacy encodings keep their numbers.
- Tests pin the rule with multibyte UTF-8, a BOM, a linked stylesheet and an encoding
  error.

**Non-Goals:**

- Changing line numbering, file names or diagnostic codes.
- Counting grapheme clusters or display columns; the unit is a Unicode code point, as
  the documentation already says.
- Changing the position of a diagnostic that has no source line.

## Decisions

**Add `charset.Column(src []byte, lineStart, offset int) int` and route every path
through it.** The function counts code points between `lineStart` and `offset`, using
`utf8.RuneCount` for UTF-8; for a legacy source the mapper supplies widths of one, so the
same function returns the byte count and existing numbers do not move. Alternative: make
`Mapper` return runes and convert every caller; rejected because `Stylesheet.Position`
and `sourcePosition` do not have a mapper and would keep a second implementation.

**Strip a leading BOM from the position origin, not from the text.** The parser already
removes the BOM before the tree, but byte offsets in the retained source still include
it, which is why the column moved by six. `Column` and the mapper treat an offset that
falls after a leading `EF BB BF` as shifted by three bytes, and the line table starts
after the mark. Alternative: drop the BOM from every source buffer; rejected because the
mapper translates decoded offsets back to the original bytes and needs the true offsets.

**Keep the per-line incremental cursor.** Counting from the line start on every call is
O(line length); the mapper already keeps a cursor, and the document path will reuse the
same idea (last line, last offset, last column). A pathological one-line document cannot
turn diagnostics into quadratic work because each call advances the cursor. Alternative:
a per-line rune-offset table; rejected as another index to bound (see
`bound-work-and-memory`).

## Risks / Trade-offs

- **Pinned columns change for multibyte sources.** Expected and intended; only the
  column moves, and the affected golden positions are updated in the same change.
- **A BOM makes the mapper's source offsets non-obvious.** The shift is applied in one
  place with a unit test for offsets before and after the mark.
- **Legacy tests keep passing for the wrong reason.** The new tests use UTF-8 multibyte
  and a BOM so byte and character columns differ.

## Migration Plan

No API change. Diagnostics for multibyte sources keep their file, line, code and message
and change only the column, which the documentation already described as characters. The
`CHANGELOG.md` `Unreleased / Fixed` entry states that a BOM no longer shifts a column and
that linked stylesheets and charset errors follow the documented rule.
