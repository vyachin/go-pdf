# Proposal

## Why

`docs/profile.md`, the README and the package documentation promise that a diagnostic
column counts characters, not bytes. Three of the four position paths count bytes: the
charset mapper, the fallback in `Stylesheet.Position` for a linked sheet with no decoder,
and `charset.sourcePosition` for an encoding error. Only `Document.Position` for a UTF-8
document without a byte order mark counts runes. Measured: a UTF-8 document with a BOM
reports a missing image at `1:20` while the same bytes without the BOM report `1:14`; a
linked UTF-8 sheet reports `1:29` for the declaration whose character column is 26; an
encoding error reports `2:13` where the author sees column 9. The encoding tests use
windows-1251, where byte and character columns coincide, so the suite cannot see the
difference.

## What Changes

- Add one rune-column implementation and use it in the document, mapper, stylesheet and
  charset position paths.
- Treat a leading byte order mark as zero-width, so adding or removing it never shifts a
  column.
- Keep legacy single-byte columns unchanged, because there one byte is one character.
- Pin the rule with UTF-8 multibyte, BOM, linked-stylesheet and charset-error tests, and
  make `docs/profile.md` and the README state the same rule the code applies.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the diagnostic-position requirement gains one character-column rule for
  every source kind, including a linked stylesheet and an encoding error.

## Impact

- `internal/charset/mapper.go`, `internal/charset/charset.go`,
  `internal/css/parse.go`, `internal/html/node.go`.
- `docs/profile.md` (Input encodings / Diagnostics), `README.md` if its sentence needs
  the same wording, `CHANGELOG.md`.
- Tests: `internal/engine/encoding_test.go`, `internal/charset/mapper_test.go`, a linked
  UTF-8 stylesheet case in `internal/css`, and the existing position goldens.
- Diagnostics for multibyte sources change column; no diagnostic code, severity or
  message changes.
