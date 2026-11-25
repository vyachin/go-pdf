# Tasks

## 1. Bound and harden the parser

- [x] 1.1 Charge the guard per lookup, subtable and scanned coverage range, and check it between lookups — verify: the amplifier fixture returns `resource/limit` under a small budget.
- [x] 1.2 Cap lookups, subtables and total scanned ranges at values independent of the font — verify: a font declaring the maximum counts is rejected cheaply.
- [x] 1.3 Convert every offset/length/count path to 64-bit arithmetic before slicing — verify: unit tests with 0x80000000 offsets pass on 64-bit and the `GOARCH=386` build test does not panic.
- [x] 1.4 Reject `cmap` format 12 groups whose range exceeds 0xFFFF — verify: the `startGlyphID: 0xFFFFFFF0` fixture returns a parse error.

## 2. Tests and documentation

- [x] 2.1 Add the amplifier, overlong-offset and format-12 fixtures as regressions — verify: each fails before the fix and passes after.
- [x] 2.2 Extend the GPOS fuzz seed set — verify: `go test ./internal/font -run '^$' -fuzz '^FuzzParse$' -fuzztime 20s` stays green.
- [x] 2.3 Document the font work cap in `docs/fonts.md` — verify: the documented bound matches the constant.
