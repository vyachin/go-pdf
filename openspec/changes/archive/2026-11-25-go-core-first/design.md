# Design

## Context

The repository contains several private implementations whose job the Go project already does. The two largest are text normalization — `internal/text/normalize.go` plus the 1,960-line generated `unicode_nfc.go`, produced by an undocumented `gen_nfc.py` — and the JPEG header parser `jpegInfo` in `internal/pdf/writer.go`, while the engine already asks `image/jpeg` for the same numbers when it enforces `MaxImagePixels`. Smaller ones are `utf16Hex` and `decodeUTF16Hex` next to a standard `unicode/utf16`, `maxInt` and `strconvQuote`/`hex4` next to the language and the standard library, and a stylesheet containment check in `internal/profile` that repeats what `fsx` already does through `os.Root`.

Two measured facts bound the change: the private composition tables are 43 KB of source and `x/text/unicode/norm` adds about 137 KB to the binary; the table version of the generated code is Unicode 17.0.0, and so are the tables in `x/text v0.44.0`.

See `proposal.md` for motivation and `code-quality` for the new rule.

## Goals / Non-Goals

**Goals:**

- Remove a private substitute wherever the Go project covers the need, without changing the output of existing documents.
- Fix the surrogate-pair bug that the comparison exposes.
- Make the exceptions explicit and reviewable, so the rule is not read as an absolute promise.

**Non-Goals:**

- Replacing the font parser. `x/image/font/sfnt` parses `cmap`, `hmtx` and `glyf`, but it does not expose the raw tables the subsetter rewrites and it cannot subset; using it would leave the private parser in place and add a second one.
- Replacing the debug raster scaler. Nearest-neighbour is a deliberate choice: `testdata/golden/render` is compared pixel by pixel, and interpolation would make every golden depend on the filter.
- The CSS grammar and the PDF syntax: no Go-project implementation exists.
- Locale-aware case mapping in the browser lowering: it is a best-effort approximation of an unsupported property and already reported as a warning.

## Decisions

### 1. Normalization becomes `norm.NFC`

`text.Normalize` delegates to `norm.NFC.String`. `unicode_nfc.go`, `gen_nfc.py`, the `combiningClasses` map and the unused `HasCombiningMark` are deleted. The package keeps the wrapper so callers do not change and the documented policy stays in one place.

*Evidence that this is safe*: all twenty vectors in `TestNormalize` — Cyrillic, Latin, two-step composition, Hangul, marks that cannot compose — already equal `norm.NFC`; so does the combining-class ordering test. The only difference found is the case the private code got wrong: `U+1E0B U+0323` composes to `U+1E0D U+0307` under NFC, while composition without prior decomposition left it untouched.

*Why not keep the tables*: they implement a subset of NFC (composition only), need regeneration when Unicode moves, and have a generator no document explains. *Why not vendor `norm`*: it would re-create the maintenance this change removes.

### 2. JPEG headers come from `image/jpeg`

`imageObject` calls `jpeg.DecodeConfig` and maps the colour model to the PDF component count: `color.GrayModel` → 1, `color.CMYKModel` → 4 (already rejected), anything else → 3. `jpegInfo` disappears. The engine and the writer then agree on which files are valid, and an undecodable file fails with a diagnostic instead of being embedded.

*Cost*: a JPEG variant Go cannot decode — 12-bit or lossless — is now rejected rather than passed through untouched. No document or test uses one, and a file the engine cannot rasterize is not one it should embed silently.

### 3. UTF-16 uses `unicode/utf16`

`utf16Hex` encodes a rune with `utf16.Encode` and formats each unit as four hex digits; the output for BMP characters is unchanged and the surrogate pair for a non-BMP character is the same pair the manual code produced. `decodeUTF16Hex` uses `utf16.Decode`, which fixes the bug: a surrogate pair used to come back as two code points, so the extracted text for an emoji or a historic script was wrong.

### 4. The standard-library sweep

| Current | Replacement |
|---|---|
| `maxInt` | the builtin `max` |
| `sort.SliceStable` in `diag` and `css` | `slices.SortStableFunc` |
| `sort.Slice` in `pdf`, `font/subset`, `raster`, `docdump` | `slices.SortFunc` |
| `sort.Strings` over map keys | `slices.Sorted(maps.Keys(m))` |
| `sort.Search` for the line of an offset | `slices.BinarySearch` |
| `strconvQuote` | `strconv.Quote` |
| `hex4` | `fmt`'s `%04X` |

The two stable sorts stay stable: `diag.List` and the cascade depend on the order of equal keys, and golden output pins it. Everything else is an equivalence, not a behavior change.

### 5. One containment rule for stylesheets

`resolveStylesheetPath` keeps the URL rules (remote, `data:`, empty) and the join with `BaseDir`, and drops its own `filepath.Abs`/`Rel` boundary check. `fsx.Root` already resolves symlinks, checks the boundary and opens through `os.Root`, and `css/link-path` is emitted from the existing mapping of `fsx.ErrOutsideRoot`. The image loader already works this way.

*Alternative*: keep both checks — rejected: two implementations of one rule diverge, and the weaker one runs first.

### 6. The policy is written down with its exceptions

The README gains one sentence: where the Go project provides a facility the engine needs, the engine uses it. `CONTRIBUTING.md` gains the list of exceptions with their reasons (font subsetter, debug raster, CSS grammar, PDF syntax, browser-lowering case mapping), so a reviewer can see whether a private implementation is justified. The `code-quality` requirement makes the rule durable and gives the exceptions a home.

## Risks / Trade-offs

- **Full NFC changes an edge sequence** → the corpus and the golden layers are checked; a changed golden is regenerated deliberately in the same commit and named in the changelog.
- **A JPEG variant is rejected** → documented in the changelog; the error names the file.
- **Sort stability** → the two stable call sites keep `SortStableFunc`; `docdump` and the golden layers pin the order.
- **The containment refactor changes an error path** → the codes stay the same and the symlink tests run. The check becomes stronger, not weaker.
- **The policy is read as absolute** → the exception list in `CONTRIBUTING.md` and the requirement's wording ("only where the module does not cover the need") prevent that.
- **137 KB for the normalization tables** → accepted; it replaces 43 KB of source and a generator.

## Migration Plan

No migration. Output for existing documents is unchanged except where the new NFC composes a sequence the old code could not, and that is a deliberate improvement. Reverting the commit restores the previous behavior and the generated tables.
