# Proposal

## Why

`parseGDEF` runs for every font that has a GPOS table, before any lookup is
selected. It expands every mark class range of class 3 into a map but charges the
work guard nothing and never checks cancellation; GDEF class ranges need not be
sorted or disjoint, so 2,000 overlapping ranges in a 12 KB font take 1.35 s with
`guard.Used() == 0`, and the format maximum extrapolates to about 43 s from a
~393 KB file, well under `MaxFontBytes`. A context cancelled 20 ms into that parse
is observed only after 1.34 s. The mark-set coverage loop also swallows the
parser's limit error and continues with a partial mark set instead of reporting
`resource/limit`. This violates the documented "Font parsing work is bounded"
requirement and makes a small hostile font a CPU denial of service.

## What Changes

- Thread the conversion's work guard into `parseGDEF` and charge per class range
  and per expanded entry before the map is written, the same way PairPos
  expansion is charged.
- Check the guard between ranges so context cancellation and budget exhaustion
  stop the parse promptly, not after the whole table.
- Cap the total expanded mark entries and return `resource/limit` when a font
  declares more, instead of building a partial mark set.
- Propagate a coverage limit error from the mark-set loop: a resource refusal is
  terminal, while a genuinely malformed optional table still degrades to "no
  GDEF" as the existing optional-table rule allows.
- Record the GDEF bounds and their numbers in `docs/fonts.md`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: GDEF parsing is covered by the font-parsing work bound and the
  cancellation requirement; coverage limits are terminal, malformed data is not.

## Impact

- `internal/font/gpos.go` (`parseGPOS`, `parseGDEF`, the mark-set coverage loop),
  `internal/font/font.go` (guard plumbing).
- Tests in `internal/font` (charged units, mid-parse cancellation, cap) and an
  adversarial fixture in `internal/engine` with a small `MaxOperations`.
- `docs/fonts.md` parsing-bounds section; `CHANGELOG.md`.
- No exported Go API change and no output change for fonts within the bounds.
