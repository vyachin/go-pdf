# Proposal

## Why

Real templates are not always UTF-8. A document exported from 1C, Word or Excel, or any file saved by a Windows tool, may arrive in windows-1251 and often starts with a byte order mark. The engine treats every input as UTF-8, so such a file is not rejected with a useful message: the bytes become U+FFFD (or a U+FEFF element for a BOM), the font has no glyph for that character, and the conversion fails with `text/glyph`, which names the wrong cause and offers no way forward.

## What Changes

- The input encoding SHALL be determined the way the HTML standard determines it: a byte order mark first, then a `<meta charset>` or `<meta http-equiv="Content-Type">` in the first 1024 bytes, otherwise UTF-8. Detection SHALL use `golang.org/x/net/html/charset`, not a private scanner.
- Only a closed set SHALL be accepted: UTF-8, windows-1251, KOI8-R and IBM866. Any other result — including UTF-16 and windows-1252 — SHALL fail with `html/encoding` naming the encoding.
- A UTF-8 BOM SHALL be stripped before parsing. A UTF-16 BOM SHALL be rejected with `html/encoding`.
- `Options.Charset` SHALL override detection for a document that declares nothing, and the reference command SHALL gain a matching `-charset` flag. An unknown value SHALL be `options/charset`, before any file is read.
- A document that is not valid UTF-8 and declares no accepted encoding SHALL fail with `html/encoding` at its first invalid byte, instead of reaching the font layer with U+FFFD.
- A byte that the selected encoding does not define SHALL fail with `html/encoding` at that byte. Silent replacement with U+FFFD is not allowed.
- An external stylesheet SHALL be decoded with the document's encoding. A `@charset` rule that disagrees SHALL be `css/link-encoding`; one that agrees SHALL be accepted and ignored.
- A diagnostic for a decoded document SHALL still point at the original file: a position inside the decoded text MUST be translated back to the byte offset, line and column of the source.
- The README dependency sentence SHALL stop counting modules and SHALL state the allowlist instead.

Not included: statistical detection (the engine never guesses); UTF-16 input; windows-1252 in automatic detection (its result is indistinguishable from "nothing declared", see design); legacy encodings for asset paths, which stay UTF-8; `@charset` as a second source of truth.

## Capabilities

### Modified Capabilities

- `html-parsing`: a new requirement for input decoding, the BOM, the closed encoding set and its diagnostics; the existing position requirement gains the guarantee that a decoded document reports source positions.
- `css-profile`: the external-stylesheet requirement gains the document encoding and the `@charset` rule.
- `reference-cli`: the usage line gains `-charset`, and a wrong value is a usage error.

## Impact

- **Code**: a new `internal/charset` (detection, the closed set, per-byte tables, offset mapping); `internal/html` (decode before the scan, positions); `internal/profile` (external stylesheets); `internal/engine` (the option and the diagnostics); `pdfgen` (`Options.Charset`); `cmd/pdfgen` (the flag).
- **Dependencies**: `golang.org/x/net/html/charset` enters the runtime graph and brings its encoding tables; `golang.org/x/text` is already required. The binary grows by about 830 KB.
- **Diagnostics**: three new codes, `html/encoding`, `css/link-encoding` and `options/charset`; no existing code changes meaning.
- **Tests**: windows-1251, KOI8-R and BOM fixtures; a detection table; position mapping; undefined bytes; unsupported declarations; invalid UTF-8; external stylesheets; determinism; the CLI.
- **Documentation**: README (input encodings and the dependency sentence), `docs/profile.md` (meta and external CSS encoding), CHANGELOG; the new binary fixtures need a `.gitattributes` rule.
- **Compatibility**: additive. Valid UTF-8 documents keep their current output byte for byte; a document that used to fail now converts or fails with a better code.
