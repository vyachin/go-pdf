# Design

## Context

The guard's unit was chosen before the stages that copy bytes existed. Today
`MaxOperations` bounds token counts, boxes, pages and drawing operations, while
`lineStarts`, the decoded document text and the painted glyph segments allocate in
proportion to input bytes without a charge. Measured with the default limits: 7 MiB of
CSS made of newlines allocates 322 MiB total / 74 MiB heap with no diagnostic, the same
size of `x` allocates 22 MiB, 60 unique 1 MiB stylesheets convert successfully with
73 MiB heap, and 8 MiB of plain text gives 1.3 GiB total allocation / 401 MiB heap with
`err == nil`. The `Bounded` guarantee and the IMP-03 cost-model requirement both say the
budget bounds memory; the measurements show it does not.

## Goals / Non-Goals

**Goals:**

- One documented bytes-per-unit rule that every byte-proportional stage charges before
  it allocates, enforced by `TestAdversarialSpendIsProportional`.
- A bounded line index whose entries are charged, with correct diagnostics past the cap.
- `Limits.MaxTotalStylesheetBytes` with a documented default, mirrored into `pdfgen`,
  `DefaultLimits`, the limits table and `docs/profile.md`.
- No second glyph-slice copy when no offset is needed and an exact-size, charged copy
  otherwise.
- Byte-identical output for documents inside the limits.

**Non-Goals:**

- Streaming the PDF (separate change `add-streaming-output`); the assembled PDF stays in
  memory here.
- Lowering `MaxInputBytes` or the default `MaxOperations`; the fix is the cost model, not
  a smaller cap.
- Exact accounting of every allocator in the engine; only stages whose cost grows with
  the input are in scope.
- Changing the diagnostic vocabulary.

## Decisions

**One `bytesPerUnit` constant, used by charges and tests.** A unit stands for a
documented 4096 bytes, the same constant the existing proportionality test uses. Every
new charge is `ceil(n / 4096) + 1`, taken before the allocation it describes. This keeps
the unit meaningful and lets the test assert an upper bound instead of guessing.
Alternative: charge every `make` call; rejected as invasive and untestable at that
granularity.

**Cap the line index instead of making it lazy.** The parsers keep the first 65,536 line
starts and, for an offset past the cap, scan from the last stored start to the offset,
charging the scanned bytes. Common documents keep exact O(1) lookups; a stylesheet made
of newlines stops at the cap with `resource/limit` before the slice grows without bound.
Alternative: a fully lazy index; rejected because positions are needed while the tree is
built and a lazy structure would change every position call site.

**Default `MaxTotalStylesheetBytes` is 16 MiB.** It admits two maximum-size (8 MiB)
sheets, which covers a print stylesheet plus a shared base, and matches the aggregate
image and font limits already in `Limits`. The field is additive; zero resolves to the
default so the zero options value stays valid. Alternative: 64 MiB to match images;
rejected because linked CSS is normally small and the measured attack needs only 60 files.

**Charge every asset read by bytes in `Registry.Read`.** The registry is the single place
every reference passes through, so one charge cannot be bypassed by a new adapter. The
byte charge is I/O cost; image pixel accounting and font parsing keep their own charges
because they bound different work. The overlap is deliberate and cheap: 64 MiB of images
costs 16,384 units of 20,000,000. Alternative: charge only stylesheets in the profile
loader; rejected because it leaves the next adapter free to reintroduce the gap.

**Reuse the shaped slice unless an offset exists.** `paintShapedRun` checks the run for a
non-zero `XOffset`/`YOffset`; without one it paints `run.Glyphs` directly, otherwise it
allocates `make([]Glyph, len(run.Glyphs))`, copies and charges one unit per glyph. This
removes the append-grown second slice that dominated the 8 MiB profile.

## Behavioral verification

Section 0 establishes tests before the corresponding stage changes. Each covered
stage has independently observable completed work and a documented charge; a
budget insufficient for the next charge must prevent the read, expansion or copy
from starting. Observe stage-local deltas instead of freezing unrelated pipeline
charges. Vary input size at a fixed budget and budget at a fixed input size to
check the documented work/allocation bound, allowing an explicit fixed overhead.
Use one fixture definition for shared preparation/layout in `Validate` and
`Convert`, and measure writer-only work separately. Counters and allocated bytes
determine correctness; wall-clock durations never gate these tests.

## Risks / Trade-offs

- **Legitimate documents spend more units.** A document near the default
  `MaxOperations` may now stop earlier. The measured 8 MiB text case charges about
  16 M of 20 M units, so the default still admits it; the invariant test pins the
  multiple, and callers who need more raise the limit.
- **A capped line index makes a position lookup on a pathological single-line document
  O(line length).** The scan is charged, and normal documents never reach the cap.
- **16 MiB of linked CSS can reject a document that converts today.** That document is
  outside the intended profile; the new diagnostic names the total, and the field is
  documented and configurable.
- **Uniform read charging overlaps existing image and font accounting.** The overlap is
  small relative to the default budget but changes recorded spend; tests that assert
  exact unit counts need recalibration.

## Migration Plan

Additive and internal. `Limits.MaxTotalStylesheetBytes` is a new field; a zero value
resolves to 16 MiB, so existing callers compile and keep converting ordinary documents.
Update `pdfgen.DefaultLimits`, the mirror test, `docs/profile.md` and the `CHANGELOG.md`
`Unreleased` section. Documents that legitimately need more than 16 MiB of linked CSS set
the field; the release note names the new limit and the default.
