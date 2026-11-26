# Design

## Context

See `proposal.md` — Why. Shaping maps logical text to glyphs and the layout places
runs left to right. Neither stage knows about bidirectional text: there is no
bidi reordering, no GSUB, and the `dir` attribute is accepted by the profile and
ignored. The verified output for Hebrew is the logical order painted left to
right, and for Arabic additionally unjoined letters, with no diagnostic.

`golang.org/x/text` is an existing, allowed dependency and already provides
`unicode/bidi`, the UAX#9 implementation. `docs/profile.md` lists `dir` among the
global attributes, so the HTML surface needs no new attribute.

## Goals / Non-Goals

**Goals:**

- Hebrew, and any other right-to-left script that needs only reordering, is
  painted in visual order and extracts in logical order.
- A script whose shaping the engine cannot perform is refused with a diagnostic
  instead of producing silently wrong output.
- Latin-only documents keep their current bytes.
- The supported script set is documented and checked against the code.

**Non-Goals:**

- Implementing GSUB, Arabic joining, Indic reordering or ligature substitution.
- Glyph mirroring for paired punctuation such as parentheses; the profile
  documents that the character's own glyph is drawn.
- CSS `direction` and `unicode-bidi`, vertical writing, or CJK line breaking.
- Reordering across a page or column boundary: each line is independently
  ordered, as UAX#9 defines for a paragraph line.

## Decisions

**Use `x/text/unicode/bidi`, not a private implementation.** The dependency is
already allowed, UAX#9 is subtle, and a private implementation would need its own
conformance corpus. The paragraph is fed the line's logical text with the resolved
base direction; its levels drive run ordering.

**Reorder runs, not individual glyphs.** A run is the unit the shaper produced and
kerning applies inside it. Splitting or reversing glyphs would break pair kerning
and the mark attachment done by the shaper. The visual order is applied when the
line's items are placed: right-to-left runs are appended from the right edge in
reverse run order, and a mixed line places each run according to its bidi level.

**Base direction precedence.** The nearest ancestor `dir` (including the element
itself) wins: `ltr` or `rtl` set the base direction, `auto` selects the first
strong character. Without `dir`, the first strong character of the paragraph
decides, then left-to-right. An unrecognized `dir` value is treated as absent;
the profile already accepts the attribute and this change does not add a new
validation rule for its value.

**Refuse joining scripts as an error, not a warning.** A warning would still ship
a wrong PDF, and the project already refuses text it cannot render (a missing
glyph is an error). The detector is a small rune-range table for scripts with
required contextual joining; the diagnostic is `text/unsupported-script` carrying
the script name and the element position. `Strict` is irrelevant because the
condition is already an error.

**Text layer stays logical.** `ToUnicode` maps CIDs to the authored code points;
visual order exists only in the content stream. Extraction therefore returns the
logical sequence, which is what a copy from a browser with bidi produces.

**Justification and alignment follow the visual line.** Extra space is
distributed between the visual runs of the line; `text-align: left/right` remain
physical directions, so no property semantics change. Margin boxes use the same
line placement path and inherit the behaviour.

## Risks / Trade-offs

- **Wrong ordering in an edge case** (brackets, neutrals at line edges) → the
  golden line is covered by fixtures for Hebrew, mixed runs and explicit `dir`;
  the algorithm is the standard library's, not a hand-written approximation.
- **New refusal rejects documents that used to "convert"** → that output was
  wrong; the release note states which scripts are refused and why. Hebrew,
  Cyrillic, Greek and Latin are unaffected.
- **Bidi work is per line and adds cost** → the pass is linear in the line length
  and the layout already charges placed lines to the guard.
- **Mirrored punctuation is not substituted** → documented as a known limitation
  rather than silently promised; a later GSUB change can add it.
- **Golden snapshots are produced by poppler** → new RTL fixtures assert glyph
  x-positions and extracted text, not new render goldens, so the pinned renderer
  does not need a version bump.

## Migration Plan

No stored data or API changes. A deployment that rendered right-to-left documents
should re-render them: the new PDF is the intended output, and a joining script
now fails with `text/unsupported-script` instead of producing an unjoined file.
The profile document and changelog are updated in the same change.
