# Proposal

## Why

The shaper keeps the last mark in `markState`/`hasMark` and never clears it when a
new base glyph arrives. In `о́о́` (U+043E U+0301 U+043E U+0301) the second acute
is attached mark-to-mark to the *first* base's mark, so it is painted over the
first `о` and the second `о` stays unaccented; `а́б́` behaves the same. Verified
with the bundled font. No diagnostic is reported, and the output silently
misrepresents the text in any language that uses a base+mark+base+mark sequence.

## What Changes

- Reset the mark state whenever a base glyph is stored, so attachment always
  starts from the current base.
- Use `MarkMark` only when the immediately preceding glyph is a mark of the same
  cluster; otherwise try `MarkBase`/`MarkLig` for the current base.
- If the font has no `MarkMark` anchor for a consecutive mark, fall back to the
  current base's `MarkBase` anchor instead of a stale mark anchor.
- Reject an anchor that would place the mark over a different base or outside the
  shaped run: report the existing missing-mark diagnostic instead of painting the
  mark elsewhere.
- Add regression coverage for `о́о́`, `а́б́`, two marks on one base, and a mark at
  the start of a run.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: the combining-mark attachment state machine is specified per
  cluster, including mark-to-mark precedence, fallback and offset validation.

## Impact

- `internal/text/shape.go` (glyph state and attachment); no exported API change.
- Tests in `internal/text` (advance and offset assertions) and
  `internal/engine` (render/position fixture for `о́о́`).
- `CHANGELOG.md` under `Fixed`.
- Existing documents whose marks are already attached correctly are
  byte-identical; only sequences with a stale mark anchor change.
