# Proposal

## Why

Encoding detection scans the first 1024 bytes for `<meta` and then takes the first
literal `charset` anywhere inside the tag slice. It therefore reads markup that is not a
declaration: `<!-- <meta charset="utf-8"> -->` followed by windows-1251 bytes is reported
as `unsupported document encoding "utf-8"` at 1:6 — naming a supported encoding as
unsupported — and `<meta name="description" content="charset is a word">` reports
`unsupported document encoding "is"`. The scan is used to tell a declared windows-1252
from the no-declaration fallback, so a comment or a description can decide how the whole
document is decoded and what the error says.

## What Changes

- Replace the substring scan with a prescan over the first 1024 bytes that skips
  comments, doctypes, processing instructions and non-`meta` tags, and parses the
  attributes of a real `<meta>` element.
- Recognise only a real `charset` attribute or an `http-equiv="content-type"` element
  whose `content` names a charset; ignore the word `charset` in any other attribute value.
- Fold ASCII case without changing byte length, so the reported `<meta>` offset and the
  column stay where the author wrote them.
- Keep the detection order unchanged: caller option, byte order mark, declaration inside
  the first 1024 bytes, then UTF-8.
- Cover the commented declaration, the description attribute, a genuine `charset`
  declaration, an `http-equiv` declaration and a BOM that beats both.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `html-parsing`: the input-encoding requirement gains a precise prescan contract for
  which declarations count and how a non-declaration is ignored.

## Impact

- `internal/charset/charset.go` (`declaredCharset`, `charsetLabel`, `firstMetaOffset`
  and their helpers).
- Tests: `internal/charset/charset_test.go`, `internal/engine/encoding_test.go`, and the
  encoding fixtures under `testdata/fixtures`.
- `docs/profile.md` encoding section states that only a real declaration counts.
- No public API change; a legacy document whose only apparent declaration was fake now
  decodes by the documented fallback instead of failing with a misleading message.
