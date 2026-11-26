# Design

## Context

`internal/pdf` is the serialisation end of the pipeline: it receives drawing
operations, subsets fonts and assembles objects. The four defects are local to
that package and independent of each other, so one change carries them and one
review covers the byte-level tests. The writer is already byte-deterministic and
validated by `pdfcpu`; the change must keep both.

## Goals / Non-Goals

**Goals:**

- Correct colour for palette and gray+alpha PNGs with partial transparency.
- A font descriptor whose flags agree with its italic angle.
- A `ToUnicode` map with no gap for an emitted CID.
- A compressor that cannot truncate a stream or leak its writer.
- Byte-level tests that fail on the current output.

**Non-Goals:**

- Changing the PNG decoder, the subsetter or the CID assignment.
- Supporting indexed colour spaces; the profile writes DeviceRGB/DeviceGray.
- Re-encoding JPEG images or changing their colour space.

## Decisions

**Unpremultiply at the sampling boundary.** Palette and gray+alpha samples are
read through `color.Color.RGBA()`, which is premultiplied by contract. The
sampling code will divide each channel by alpha (for `a > 0`) before writing the
colour byte, keeping the existing `/SMask` path. Alternative rejected: re-decode
every palette image into `*image.NRGBA` first — it doubles peak memory for the
common case and changes the shared `pngToSamples` shape for one branch.

**Flags derive from the same value that writes `/ItalicAngle`.** The descriptor
will compute `flags := 4; if angle != 0 { flags |= 64 }` from
`fe.font.ItalicAngle()`. Alternative rejected: read the `post` table's italic
bit — it may disagree with `ItalicAngle`, producing an internally inconsistent
descriptor, and the PDF consumer reads both.

**Map U+0000 instead of rejecting it.** The writer's contract is that every
emitted CID is extractable; filtering `r != 0` silently breaks it. The CMap will
emit `<CID> <0000>`. Alternative rejected: reject a NUL rune at the writer — the
engine never produces one today (normalisation maps NUL to U+FFFD), so the
rejection path would be untested and would turn a hypothetical input into a
conversion failure without a documented code.

**Always close, then choose the body.** `streamObject` will write, close inside a
`defer`-equivalent that records the error, and only then compare lengths. If
either write or close fails, it returns the uncompressed body. Alternative
rejected: propagate the error — a compression failure is a size optimisation,
not a reason to lose the document; falling back keeps output valid, and the
error is visible through the seam test.

**Test the failure path with an injected encoder.** `streamObject` will take the
compressor as a small function parameter (or call a package-private helper), so a
test can supply a failing writer without a mutable global. Alternative rejected:
a package-level hook variable — it reintroduces mutable global state on the
conversion path, which the package promises not to keep.

## Risks / Trade-offs

- **Palette regression for fully opaque images** → covered by an explicit
  opaque scenario asserting identical bytes and no mask.
- **An unpremultiply divide by zero** → alpha zero is handled as fully
  transparent; the colour bytes are irrelevant and are written as zero.
- **Golden render snapshots move** → expected only for documents with palette or
  gray+alpha transparent PNGs; the change lists the regenerated files and reviews
  the diff, and the PDFs stay valid under `pdfcpu -m strict`.
- **A stale `/Flags` assumption elsewhere** → no test asserts `4` for an italic
  face today; the new assertion is the only one.

## Migration Plan

None for callers: no exported identifier, option or diagnostic changes. The
release note goes under `Fixed`; documents whose embedded palette/gray+alpha
images were darkened now render the source colour, which is a correction rather
than a format change.
