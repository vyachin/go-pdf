# Proposal

## Why

Five validation defects let the profile accept values it does not implement, or hide
where a problem is:

- A negative or zero `line-height` is rejected only for the absolute `MP` unit.
  `line-height: -10%` and `-2em` validate, produce a negative computed line height,
  and layout silently replaces it with an ascent-derived value, so the page is not
  what the CSS says.
- The initial border colour is black instead of CSS's `currentColor`, and the
  `currentColor` keyword itself is rejected. `color:#c00; border:1px solid` paints a
  black border, not a red one.
- A rejected declaration in a `style` attribute reports `:0:0`, although the owning
  element's position is known, so an editor cannot jump to the problem.
- A page margin box such as `@top-center` is accepted outside `@page` and painted,
  which is not a construct the profile defines.
- The problem cap is double-counted: a problem recorded inside a block is counted
  again when the block is merged, so 60 bad declarations are truncated at 40 and
  reported as `resource/limit` naming a cap of 100.

## What Changes

- **BREAKING** for a template that relies on the current leniency: a negative or
  zero percentage or `em` `line-height` now fails validation, and a margin box
  outside `@page` is now rejected. A border declared without a colour now takes the
  element's `color` instead of black, so its output changes.
- Reject a `line-height` that resolves to zero or below in every unit.
- Make `currentColor` the initial border colour and accept the keyword.
- Attach the owning element's position to every diagnostic raised for an inline
  declaration.
- Reject a page margin box at-rule unless it is a direct child of `@page`.
- Count each parser problem once against the cap.
- Add unit, profile and public-API tests for each case.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: colour and `line-height` validation, inline diagnostic positions,
  margin-box placement and the problem cap each gain a specific requirement.

## Impact

- `internal/css/props.go` (`line-height` parsing, border initial value).
- `internal/css/value.go`, `internal/css/style.go` (`currentColor` resolution).
- `internal/css/cascade.go`, `internal/css/parse.go` (inline positions, problem cap).
- `internal/profile/profile.go` (margin-box placement, inline fallback position).
- `internal/css/style_test.go`, `internal/css/harden_test.go`,
  `internal/css/nesting_test.go`, `internal/profile/profile_test.go`,
  `internal/engine/convert_test.go`.
- `docs/profile.md` colour and `line-height` rows.
- No new diagnostic code; the existing codes carry the cases.
