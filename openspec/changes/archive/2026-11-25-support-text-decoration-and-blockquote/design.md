# Design

## Context

See `proposal.md` for the finding. The profile is a closed allow-list, so an
element or property that is not in `internal/css` is a validation error; adding a
construct therefore means a profile entry, a default rule and a painted
representation, never a silent pass-through.

## Goals / Non-Goals

**Goals:**

Render the four text-decoration elements and blockquote from the same
declarative machinery every other construct uses, so validation, layout and the
snapshot stay consistent.

**Non-Goals:**

Adding `text-decoration-style`, `text-decoration-color`, `text-decoration-line`
as a separate longhand, `<q>`, or arbitrary quote nesting; no new public API and
no dependency.

## Decisions

1. Model the decoration as an inherited bit set on `css.Style`. CSS propagates a
   decoration from the decorated box to its descendants, so inheriting the flag
   matches the painted result for the constructs the profile supports, and it
   avoids collecting an ancestor chain at paint time.

2. Parse `text-decoration` as `none`, `underline`, `line-through`, or the two
   keywords in either order. Anything else — `blink`, `overline`, a colour, a
   style keyword — is a profile error naming the value, which keeps the rule
   "the profile is closed" true.

3. Paint one line per contiguous decorated run, not per glyph: layout knows the
   run's start, width and baseline, so the paint pass appends a filled rectangle
   in the run colour. Underline sits below the baseline by a fraction of the font
   size, line-through crosses at the middle of the x-height; both use the same
   thickness rule, so a decorated word is one rectangle rather than a rain of
   thin boxes.

4. Give `blockquote` block layout with a left and right indent from the engine
   stylesheet, and `u`/`ins` underline while `s`/`del` strike through. The
   elements stay ordinary inline/block boxes, so anchors, margins and the box
   tree need no special case.

5. Keep `text-decoration` out of the inherited-property list of the snapshot
   model? No: the snapshot records the computed style, so the flag appears there
   and the fixture proves both layers agree.

## Risks / Trade-offs

The paint geometry is a deliberate approximation: the profile has no
`text-decoration-thickness`, so a fixed fraction of the font size is used and
documented. A decorated run that wraps produces one line per run rather than one
continuous line across the whole paragraph; the supported constructs do not need
more.

## Migration Plan

Land the profile entries, the engine-stylesheet rules, the parser, the painting
and one snapshot fixture together. Existing documents are unaffected: no UA rule
changes for an element that was already supported, and a template that used
`<u>` or `<blockquote>` previously failed validation and now renders. Revert the
implementation commit and its snapshot update together if a regression is found.
