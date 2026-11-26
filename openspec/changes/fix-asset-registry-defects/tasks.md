# Tasks

## 0. Behavioral invariant tests before implementation

- [ ] 0.1 Instrument the actual font-parser entry point with an instance-local
  test seam and strengthen `TestDeclaredAndFallbackShareOneFont` and
  `TestFontSpellingsAndFallbackUseOneRecord`. Exercise canonical path aliases,
  multiple declared families, explicit fallback and repeated size requests.
  Verify one successful read, one actual parse and one byte admission per
  canonical font in a conversion; shared pointers and cache size alone are not
  evidence of one parse. Two distinct canonical fonts must each be parsed once.
- [ ] 0.2 Make the eight-reader test deterministic with a barrier around the
  first underlying read, and count actual reader invocations as well as registry
  statistics. Verify all peers share one read and one admission under the 1.5 MiB
  total, with channels rather than sleeps controlling the overlap. Check that
  the same counters are independent in two conversions under `-race`; parser
  assertions cover supported adapter paths without requiring parallel parsing.
- [ ] 0.3 Count the failing read/parse attempt and a later retry separately.
  Verify a failure leaves no successful cache entry or retained charge, a retry
  invokes the failed stage again, and successful later reuse adds no extra parse
  or charge. Keep instrumentation local to the registry/adapter instance, with
  no mutable global override, environment switch or public API addition.

## 1. One read, one charge

- [ ] 1.1 Replace the cache-miss-then-reserve sequence with a per-key in-flight call that the first reader creates and later readers wait on, publishing the record under the existing mutex. Verify with eight goroutines reading one 1 MiB file against a 1.5 MiB total: all succeed, one read is recorded and the total is 1 MiB.
- [ ] 1.2 Deliver an in-flight failure to every waiter with the same typed cause, and remove the in-flight entry when the call ends so the next caller retries. Verify that a failing read leaves the cache and the totals unchanged and that a retry can still succeed.
- [ ] 1.3 Charge `len(bytes)` after the read, refund a shortfall at once, and check a surplus against the remaining total before admitting it; store the charged amount on the record and use it for the refund. Verify with a file that reports size 0 and yields 1 MiB: either refused or charged 1 MiB, never 1 MiB with a zero charge.
- [ ] 1.4 Reconcile the totals on discard so a failed decode returns the total to its previous value and never below zero. Verify with a decode failure after admission and with two failures in a row.

## 2. Classification and option order

- [ ] 2.1 Classify an open failure by re-running the containment check: only a reference that leaves the root becomes a path escape; every other cause keeps its own error. Verify that a mode-000 file named `escapes.css` inside the root reports `css/link-load`, `../outside.css` reports `css/link-path`, and a symlink leaving the root reports `css/link-path`.
- [ ] 2.2 Move the metadata validation into the option gate, before stylesheets and fonts are opened, and keep the existing invalid-option path for inline styles. Verify that `Options{Title: "bad \xff"}` plus a missing stylesheet reports `options/title` and does not open the stylesheet.
- [ ] 2.3 Keep `options/author`, `options/subject` and `options/keywords` in the same gate and confirm a valid metadata set does not change the existing `css/link-load` report for a missing stylesheet. Verify all four codes with one table test.

## 3. Release record

- [ ] 3.1 Add a `Fixed` entry to `CHANGELOG.md` naming the four behaviours: one charge under concurrency, actual-byte accounting, containment-based classification, and the option-first order. Verify the entry names no removed API and no new diagnostic code.
- [ ] 3.2 Confirm the diagnostic catalogue and profiles still agree after the code-selection change. Verify with `go test ./internal/diag ./internal/profile ./internal/assets ./internal/engine`.

## Workflow follow-up

- Run `openspec validate fix-asset-registry-defects --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/library-api/spec.md` and archive the change after review.
