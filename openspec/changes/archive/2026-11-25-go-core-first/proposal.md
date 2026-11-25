# Proposal

## Why

The engine still carries private implementations of facilities the Go project already provides: a 1,960-line Unicode composition table with a generator no document mentions, a JPEG marker parser, UTF-16 conversion helpers and small utilities the standard library has. Each one is code that must track a moving standard, and one is already wrong: the debug extractor returns a surrogate pair as two code points. The project rule is simple — use the Go project's implementation when one exists — and applying it removes both the maintenance and this class of bug.

## What Changes

- Text normalization SHALL come from `golang.org/x/text/unicode/norm`; the generated composition table, `gen_nfc.py` and the unused `HasCombiningMark` SHALL be deleted.
- JPEG width, height and colour components SHALL come from `image/jpeg.DecodeConfig`; the private marker parser SHALL be deleted.
- PDF string and ToUnicode encoding and the debug extraction SHALL use `unicode/utf16`; the surrogate-pair bug in the extraction MUST be fixed.
- Hand-written substitutes for standard-library facilities SHALL be replaced: the builtin `max`, `slices`, `maps`, `strconv.Quote` and `fmt`'s `%U`; the stylesheet path check SHALL reuse `fsx` and `os.Root` instead of a second containment check.
- The rule SHALL be recorded: the README states that the Go project's implementation is used where one exists, CONTRIBUTING lists the justified exceptions, and the `code-quality` capability gains a requirement.
- Not included: the font parser (`x/image/font/sfnt` does not expose raw tables or subsetting, so it stays a test oracle), the debug raster scaler (nearest-neighbour is deliberate: the render goldens are compared pixel by pixel), the CSS grammar and the PDF syntax (no Go-project implementation), and locale-aware case mapping in the browser lowering, which is best-effort by design.

## Capabilities

### Modified Capabilities

- `code-quality`: a new requirement that the Go project's implementation is preferred, and that every remaining private substitute is named with its reason.

## Impact

- **Code**: `internal/text` (normalization, deleted tables and generator), `internal/pdf` (`jpegInfo`, `utf16Hex`), `internal/pdfdebug` (`decodeUTF16Hex`), `internal/profile` (containment through `fsx`), and the sort and helper call sites across `internal/`.
- **Behavior**: normalization becomes full NFC, so a composed character followed by a combining mark composes where it did not before; a JPEG variant the standard library cannot decode is rejected instead of embedded; a non-BMP character in a ToUnicode map extracts as one rune.
- **Binary**: about 137 KB for the normalization tables; the deleted code is smaller than what replaces it.
- **Tests**: the normalization table already matches `norm.NFC`; new cases for full composition, the surrogate fix, JPEG variants and golden determinism.
- **Documentation**: README (the policy), CONTRIBUTING (the exceptions), CHANGELOG.
