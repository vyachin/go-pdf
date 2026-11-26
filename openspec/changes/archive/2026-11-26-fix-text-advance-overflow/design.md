# Design

## Context

See `proposal.md` — Why. Geometry is integer millipoints (`int32` positions,
sizes and advances) by design, and the layout already has `satAdd`/`satSub` for
box geometry. The text path was not converted: `AdvanceWidth` sums `int32`
advances with `+=`, `Line.Natural` is an `int32` sum, and the overflow check
compares that natural width with the content width. When the sum wraps negative
the comparison is false, so the diagnostic never fires. `Scale` multiplies
`float64` values and converts straight to `int32`, which is implementation-defined
when out of range.

## Goals / Non-Goals

**Goals:**

- No negative run, line or glyph advance from an in-range document.
- Saturation is visible as the existing `layout/overflow` warning, once per box.
- Identical diagnostics and geometry on 32-bit and 64-bit builds.
- Existing documents keep their exact output.

**Non-Goals:**

- Moving all geometry to `int64`. The public and layout representation stays
  `int32` millipoints; only the arithmetic that can leave the range is widened.
- Changing the `layout/overflow` code or adding a new diagnostic code.
- Charging saturation to the work guard: the cost is the shaped glyphs, already
  charged by shaping.

## Decisions

**Accumulate in `int64` and clamp at the boundary.** `AdvanceWidth` gains an
`int64` accumulator and returns a clamped `int32`; `Run` carries a `Saturated`
flag (or an equivalent pair) so the caller can distinguish "exactly MaxInt32" from
"clamped". The same helper computes `Line.Natural`. `paintShapedRun` already
saturates with `satAdd`, so layout and painting agree instead of one wrapping and
the other saturating.

**Propagate saturation to the existing notice.** `placeText` already warns when a
line is wider than the content box. The condition becomes
`line.Saturated || int64(line.Natural) > int64(width)`, which keeps one warning
per box through the existing `overflowWarned` guard and reuses the position and
element path. No new code, no new wording family.

**Clamp in one helper used by `Scale`, `scaleFontUnits` and `MetricsOf`.** The
helper takes `float64`, rounds once with `math.Round`, and clamps to
`[-MaxInt32, MaxInt32]`. `scaleFontUnits` already clamps its `int64` input; it
can call the same helper for the final conversion so there is one rule.

**Diagnose a clamped face once, at face resolution.** A font size large enough to
saturate an advance also saturates the face ascent/descent. `layout.face` (or the
point that builds `FontFace`) compares the requested size with the range the
face's metrics allow and records a `layout/overflow` warning at the first element
that uses the face. This avoids one warning per glyph and keeps the message
attached to a real node.

**Verify on 386 in CI.** The float-to-integer conversion behaviour is the reason
the finding exists; a `GOARCH=386 go test` job (or at least `go build` plus the
range tests) makes the architecture-independence claim testable. The unit test
also asserts the arm64 value on the current host.

## Risks / Trade-offs

- **A saturated width is still clipped** → intended: the document is invalid by
  size, the warning names it, and the PDF stays structurally valid.
- **Warning duplication** → the existing per-box `overflowWarned` flag is reused;
  the test asserts exactly one diagnostic per paragraph.
- **Clamped metrics could hide a design error** → the warning is emitted once when
  clamping happens, so it is visible rather than silent.
- **Changed output for extreme sizes** → only documents that previously rendered
  wrapped geometry change; ordinary documents are byte-identical.

## Migration Plan

No API or stored-data change. Documents with extreme font sizes now report
`layout/overflow` and may render with clamped geometry instead of wrapped values;
the changelog notes the diagnostic. No action is required for documents in range.
