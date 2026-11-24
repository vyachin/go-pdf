# Design

## Context

See proposal.md. Current writer:

- builds `imageNames map[*Image]string` and then iterates the map when writing `/XObject` resources, so page dictionaries differ between runs;
- emits text as `BT ... <glyph ids> Tj ET`, using `/W` widths only.

Layout produces `layout.Glyph` with an `Advance` that already includes kerning (kerning is folded into the previous glyph's advance by the shaper), but `engine.toPDFOps` copies only `GID` and `Rune`.

## Goals / Non-Goals

**Goals:**

- Byte-identical output for repeated runs with several images.
- PDF glyph positions equal to layout positions, including kerning.
- Text extraction keeps returning the same characters.

**Non-Goals:**

- Removing unused images from per-page resources.
- Recomputing layout from the PDF; the PDF is the output, layout stays the source of truth.

## Decisions

### 1. Deterministic resource order from the image slice

Resources are written by iterating the ordered `images` slice, which is built in first-use order over pages, instead of the name map. The map stays for name lookup inside the content stream.

*Why not sort keys*: sorting makes output stable but arbitrary; first-use order matches document order and is what a reader would expect in a diff.

*Why not per-page filtering*: it changes resource semantics beyond the reviewed defect. It is a valid follow-up, not part of this fix.

### 2. Kerning via `TJ`, advances carried through the op model

`pdf.Glyph` gains `Advance`, and `engine.toPDFOps` copies it. The writer emits one `TJ` array per text run:

```
[ <0041> -12 <0056> ] TJ
```

For every glyph except the last, the adjustment is

```
n = round((nominalMP - advanceMP) * 1000 / size)
nominalMP = round(round(units * 1000 / upem) * size / 1000)
```

A positive `n` moves the next glyph left, which tightens the pair, exactly what negative kerning does. The nominal width is computed with the same rounding as the `/W` array, so the viewer's advance and our adjustment cannot disagree by a rounding unit.

*Alternative: one `Tm` per glyph*: no reader change needed, but each glyph becomes its own `BT/ET` block, inflating the file and obscuring the text layer. Rejected.

*Alternative: rely on `/W` centred kerning*: impossible, `/W` is per glyph, not per pair.

### 3. Reader accepts both operators

`internal/pdfdebug` is a test and debugging reader. It gains support for `TJ` arrays: hex strings inside `[ ... ] TJ` are decoded in order with their adjustments ignored for extraction. Plain `Tj` stays supported for older files.

*Why not drop `Tj` support*: the package reads files produced by previous versions, and its contract is "text of a PDF produced by this engine".

### 4. Verification

- A writer test serialises a document with three images thirty times and compares bytes.
- A writer unit test builds a run with a synthetic advance smaller than nominal and asserts a positive `TJ` number.
- An engine test uses a real kerning pair from the test font, asserts the adjustment exists, and compares the effective advance with layout within one thousandth of an em.

## Risks / Trade-offs

- **Sign errors** would visibly shift text. → The unit test pins the sign; the engine test compares with layout.
- **Rounding drift** at very small sizes may leave a one-unit offset. → Nominal width uses the same integer formula as `/W`; the engine test allows one thousandth of an em.
- **Reader regressions** on `TJ`. → Extraction tests run over the existing corpora (documents, mpDF fixtures), which all use text runs.
