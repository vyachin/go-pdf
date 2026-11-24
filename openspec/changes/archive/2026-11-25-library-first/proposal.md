# Proposal

## Why

The engine is already a Go library — `pdfgen` is a public package, `internal/`
hides the implementation, and the module has no dependencies — but every
artifact still presents it as a converter binary. A converter is judged by
document fidelity and command-line ergonomics; a library is judged by whether
another Go program can depend on it with confidence: a stated contract, tested
guarantees, a licence, a version, and documentation written for the person who
calls it.

None of that exists yet. The repository has no licence, no version tag and no
stability policy. Its only runnable consumer, `cmd/pdfgen`, does not use the
public package at all: it imports `internal/engine`, `internal/fsx` and
`internal/profile` directly, so nothing demonstrates — or tests — that the
public API is sufficient for a real program. Meanwhile the command carries a
full flag surface, a browser-import narrative and a performance table that read
as the product and hide the library underneath.

## What Changes

- **The reference CLI becomes a minimal example.** `pdfgen [-o out.pdf]
  <input.html>`: one positional argument, one optional flag, no subcommands.
  It calls `pdfgen` and nothing else, and derives the asset roots from the input
  file's directory so that relative images and fonts resolve.
- **BREAKING (CLI only):** the `convert` and `validate` subcommands and the
  flags `-print-profile`, `-render-png`, `-strict`, `-date`, `-page`, `-margin`,
  `-font-dir` and `-base-dir` are removed. The command is an example; it carries
  no compatibility promise, and changing it is not a breaking change to the
  project.
- **A CI check enforces the boundary:** the direct imports of `cmd/pdfgen` must
  not contain any `internal/` package.
- **The library's unstated guarantees become explicit and tested:**
  independent conversions may run concurrently; arbitrary input never panics;
  a conversion writes nothing to disk. Each guarantee gets a test or fuzz
  target, and only then is it documented.
- **Documentation is organised by audience.** The README answers "should I use
  this?"; godoc answers "how do I call it and what is promised?"; `docs/`
  keeps the template-facing reference (`profile.md`, `fonts.md`,
  `browser-import.md`); a new `CONTRIBUTING.md` becomes the entry point for
  maintainer material (architecture, tests, golden snapshots, mPDF corpus,
  lint, performance).
- **Godoc examples are limited to one per concept** — `Convert`, `Validate`
  and error handling — instead of touring every capability.
- **Release hygiene a library needs is added:** `LICENSE`, `CHANGELOG.md`, a
  stated versioning policy, and the first version tag.
- **The misleading `examples/` directory is removed** or folded into
  `testdata/`; it currently holds one template that nothing references.

## Capabilities

### New Capabilities

- `library-api`: the contract an embedding Go program relies on — the public
  surface and its stability, no writes to disk, panic containment on arbitrary
  input, concurrent conversions, and stable diagnostic codes.
- `library-docs`: documentation as a deliverable — what the README must answer,
  godoc coverage of every exported identifier, the limit on examples, the
  contributor documentation split, and the licence/version/changelog material a
  consumer needs before adopting.
- `reference-cli`: the example command's behaviour — minimal invocation, use of
  the public package only, no compatibility promise, diagnostics on stderr and
  documented exit codes.

### Modified Capabilities

- `browser-import`: the requirement that mentions `pdfgen convert`,
  `pdfgen validate` and `convert -strict` describes a command surface this
  change removes; it must state what the reference CLI does with the import
  instead of what subcommands do.
- `pdf-output`: the "Atomic result writing" requirement claims the engine
  writes the result to a destination. The library writes nothing; the write is
  the caller's act, and only the reference CLI needs a stated behaviour for it.
- `code-quality`: "Documented local workflow" requires the README to carry the
  formatting and linting instructions; that material moves to `CONTRIBUTING.md`
  so the README can stay a front page.

## Impact

- `cmd/pdfgen`: rewritten to roughly 70 lines; its tests rewritten.
- `pdfgen`: package documentation rewritten; three guarantee tests and one
  fuzz target added. The exported API is unchanged — no symbol is added or
  removed.
- Documentation: README rewritten; `CONTRIBUTING.md`, `CHANGELOG.md` and
  `LICENSE` added; `docs/golden.md` and `docs/mpdf-corpus.md` stay in place but
  are linked from `CONTRIBUTING.md` rather than the README.
- CI: an import-graph check for the example, and `FuzzConvert` added to the
  existing fuzz step.
- No engine behaviour changes: the CSS profile, layout, pagination and PDF
  serialisation are untouched, so no golden snapshot moves.
