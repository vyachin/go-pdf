# Proposal

## Why

`GPOS` parsing is a lookup-by-subtable cross product: the LookupList count and each
lookup's subtable count are independent `uint16`s, every subtable may point at the
same PairPos, and the only bound is charged when a Coverage succeeds. A crafted
131 KB font takes 25.3 seconds in `parseGPOS`; 4 lookups × 40,000 subtables take 14.2
seconds, and the shape extrapolates to hours under `MaxFontBytes` 16 MiB. The work
guard is charged once per file, and parsing ignores the context. Two further defects
let a corrupt font escape validation: 32-bit builds overflow `cmap` and CFF offsets
and panic, and `cmap` format 12 arithmetic wraps so a crafted `startGlyphID` selects
arbitrary glyphs.

## What Changes

- Font parsing SHALL charge one unit per lookup, subtable and scanned coverage range
  and stop with `resource/limit` when the budget is exhausted.
- Lookup, subtable and scanned-range counts SHALL be capped independently of the
  values the font declares.
- Every font-declared offset, length and count SHALL be evaluated in 64-bit
  arithmetic before it is converted or used to slice, on every architecture.
- A `cmap` format 12 group whose `startGlyphID + count` exceeds 0xFFFF SHALL be
  rejected.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: font parsing is bounded and its arithmetic never overflows.

## Impact

- **Code**: `internal/font/gpos.go`, `internal/font/font.go`,
  `internal/font/cff.go`, `internal/font/cff_charstring.go`.
- **Output**: hostile fonts fail with `resource/limit` or a parse error; normal fonts
  are unchanged.
- **Tests**: the amplifier fixture, a 386 build check, format-12 overflow cases and a
  GPOS fuzz seed.
