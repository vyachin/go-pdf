# Tasks

## 0. Behavioral invariant tests before implementation

- [ ] 0.1 Add an independent expected-cost fixture for narrow, wide and
  overlapping class-3 ranges. Count completed range/entry expansion work and
  observe the guard delta; repeated writes to one glyph must still cost work.
  Verify a budget just below the next range-plus-entry charge prevents that
  expansion, the exact budget admits it, and no partial mark set is returned
  after refusal. The expected cost must not call the production charge helper.
- [ ] 0.2 Add deterministic cancellation coverage for both class expansion and
  mark-set coverage, using a controlled context/checkpoint after parsing has
  begun. Verify cancellation stops remaining work, returns `context.Canceled`
  without a font, and remains distinct from `resource/limit` through the shared
  fixture for `Validate` and `Convert`. Verify tests fail if cancellation checks
  are removed, without a sleep or elapsed-time threshold.

## 1. Guard plumbing and charging

- [ ] 1.1 Change `parseGDEF` in `internal/font/gpos.go` to accept the conversion's guard, charge one unit per class range and the entry count before each expansion, and call the guard's cancellation check between ranges. Verify a unit test that `guard.Used()` grows with the range entry count for a 2,000-range fixture and is non-zero after `LoadWithOptions`.
- [ ] 1.2 Verify a 12 KB font with 2,000 overlapping class-3 ranges stops with `resource/limit` under a small budget instead of taking about 1.35 s, and that a normal DejaVu font still loads with no diagnostic and a negligible charge.
- [ ] 1.3 Wire cancellation checks to the controlled mid-parse fixtures from 0.2. Verify the load returns `context.Canceled`, no font is returned, and no expansion begins after the next required checkpoint; do not use a timer or a wall-clock pass threshold.

## 2. Caps and error propagation

- [ ] 2.1 Add the expanded-mark-glyph cap next to the existing coverage cap, count every entry written (including overwrites by overlapping ranges), and return `resource/limit` when the cap is crossed. Verify a fixture at the cap boundary: at the cap loads, one entry over fails, and no partial mark map is returned.
- [ ] 2.2 Propagate `resource/limit` from the mark-set coverage loop in `parseGDEF` and abort the font load; keep the non-limit error path degrading to "no mark data". Verify with a mark set over the coverage cap (`resource/limit`) and a truncated GDEF (font loads, no diagnostic).
- [ ] 2.3 Add an adversarial fixture to `internal/engine/adversarial_test.go` (GDEF class-range amplifier with a small `MaxOperations`) asserting `resource/limit`, and verify `TestAdversarial` and `TestAdversarialManifest` pass.

## 3. Documentation and verification

- [ ] 3.1 Extend the "Parsing bounds" section of `docs/fonts.md` with the GDEF charge, the expanded-entry cap and the terminal-versus-degraded rule. Verify the numbers match the constants and the docs test (if any) still passes.
- [ ] 3.2 Add a `Fixed` entry to `CHANGELOG.md` stating that GDEF parsing is charged, capped and cancellable, and that a coverage resource limit is no longer swallowed.
- [ ] 3.3 Run `gofmt -l`, `go vet ./...`, `go test -shuffle=on ./...`, `go test -race ./internal/font ./internal/engine` and `openspec validate fix-gdef-parse-budget --strict`; verify all pass.

## Workflow follow-up

- Sync the delta into `openspec/specs/text-shaping/spec.md` and archive the change
  after review.
