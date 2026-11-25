# Design

## Context

`parseGPOS` reads the LookupList count, then every lookup, then every subtable of each
lookup, and each subtable may point at the same PairPos. The coverage limit only
counts successful coverage, so a failing coverage is free work. All offsets are
`uint32`; the code converts to `int` before bounds checks.

## Goals / Non-Goals

**Goals:** parsing cost bounded by the file size and a hard cap, not by declared
counts; no 32-bit overflow; a rejected cmap group cannot select arbitrary glyphs.

**Non-Goals:** implementing the full OpenType layout feature model; changing kerning
output for valid fonts.

## Decisions

- Cap lookups and subtables at 4096 each and total scanned coverage ranges at
  1,000,000; reject with `resource/limit` when exceeded. These are far above real
  fonts (DejaVu has two lookups).
- Charge the guard per lookup, subtable and range scanned, including a failing
  coverage, so a small file cannot buy large work.
- Add a small `readOffset`/`addUint` helper that checks in `uint64` before converting
  to `int`; use it everywhere an offset is sliced.
- Validate format 12 groups with `uint64(startGlyphID) + uint64(count) <= 0xFFFF` and
  reject the group, not the whole font, when only one group is bad.

## Risks / Trade-offs

- A cap that is too low rejects a legitimate complex font → pick values 10× above the
  largest font in the corpus and test with DejaVu plus a CJK face.
- Charging every range could make a large but valid font exceed the default budget →
  charge a cheap constant per range, and measure a Noto CJK font in a benchmark.
