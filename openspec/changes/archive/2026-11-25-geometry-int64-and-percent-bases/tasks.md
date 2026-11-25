# Tasks

## 1. Remove the overflows

- [x] 1.1 Compute image intrinsic size and the writer's scale in 64-bit with a range check — verify: the 3,000,000×2 PNG yields no negative `cm` and no diagnostic regression.
- [x] 1.2 Use saturating int64 helpers for every geometry sum (x accumulation, paddings, borders, `coverRect`) — verify: the two-padding case reports a diagnostic instead of painting at a wrapped X.
- [x] 1.3 Honour `width: 0` and clamp only negative or over-wide values — verify: a zero-width block paints nothing.

## 2. Resolve percentages correctly

- [x] 2.1 Carry the containing-block width on the box and use it for `width`, margins, padding and `text-indent` — verify: the four percentage scenarios in the spec pass.
- [x] 2.2 Add the percentage, wide-image and zero-width regressions — verify: each fails before the fix and passes after.

## 3. Page margins and orientation

- [x] 3.1 Resolve `@page` `em` against the initial font size and reject `auto`, `%` and negative margins — verify: the four page-margin scenarios pass.
- [x] 3.2 Flip a landscape fallback for `size: portrait` — verify: the MediaBox is 566.929×850.394 for the spec's case.
- [x] 3.3 Document the page-margin grammar in `docs/profile.md` — verify: the documented rule matches validation.
