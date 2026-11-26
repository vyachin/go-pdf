# Proposal

## Why

The tokenizer decodes escapes, and the selector parser then re-reads the decoded
text as selector syntax, so an escaped character that is also selector punctuation
changes the meaning of the selector: `.a\.b` becomes `.a.b` (two classes) and never
matches `class="a.b"`, while `.a\ b` becomes a descendant selector instead of one
class name containing a space. Separately, `url()` keeps the raw argument, so
`url("a\20 b.png")` asks the asset loader for a file literally named `a\20 b.png`
instead of `a b.png`, and an unquoted `)` cannot be escaped at all. Both cases are
valid CSS that the profile rejects or applies wrongly, and neither is reported.

## What Changes

- Preserve selector structure across escape decoding: a decoded character is data,
  never re-read as a combinator, a class separator or a pseudo-element marker.
- Decode escapes in `url()` arguments, quoted and unquoted, and let an unquoted
  `\)` close the argument.
- Add unit, profile and public-conversion tests for escaped class names and escaped
  image paths.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: selector matching and `url()` resolution each gain a requirement
  that CSS escapes are decoded to characters and never re-interpreted as syntax.

## Impact

- `internal/css/token.go` (URL token scanning and escape decoding).
- `internal/css/selector.go`, `internal/css/props.go` (selector construction and
  `url()` use).
- `internal/css/css_test.go`, `internal/css/image_hints_test.go`,
  `internal/css/harden_test.go`.
- `internal/engine/convert_test.go` for a rendered escaped selector and image.
- No public API or diagnostic code change. A template that relied on the misparse
  can match differently; this is noted in `CHANGELOG.md` under `Fixed`.
