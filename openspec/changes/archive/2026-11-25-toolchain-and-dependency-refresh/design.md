# Design

## Context

See `proposal.md` — Why, and the `code-quality` delta for the requirements. Only the facts that constrain the approach are repeated here.

- `go.mod` declares `go 1.24` with zero requirements and no `go.sum`; CI takes its toolchain from `go-version-file: go.mod`, so the directive is the single source of the CI Go version.
- The installed toolchain is `go1.27.4`; `go.dev/dl` reports it as the current stable release.
- Go requirements of the coupled validator: `pdfcpu` v0.11.0 → `go 1.23.0`, v0.16.4 (newest) → `go 1.26.0`.
- Current pins: `checkout@v7`, `setup-go@v7`, `golangci-lint-action@v9.3.0` (latest releases v7.0.1, v7.1.0, v9.3.0), `golangci-lint` v2.15.0 (latest), `upload-artifact@v4` (latest v7.0.2), `pdfcpu@v0.11.0` (latest v0.16.4).
- Baselines taken before planning: `go build`, `go vet`, `go test ./...` green on go1.27.4; `golangci-lint` 0 issues; every generated PDF already passes `pdfcpu` v0.16.4 in strict mode, because the test helper falls back to `/tmp/gobin/pdfcpu` when the validator is absent from `PATH`.

## Goals / Non-Goals

**Goals:**

- Select the lowest `go` directive that clears the newest validator, so the floor rises once and only as far as a tool requires.
- Leave the CI contract intact: the test job still proves the declared minimum, and the golden steps still pin their rasterizer.
- End with a working tree where `git status` is meaningful again, after — not before — verification has run.

**Non-Goals:**

- No new CI job, no change to test selection, no golden regeneration.
- No `toolchain` directive: the module declares a floor, it does not dictate the toolchain of whoever builds it.
- No adoption of `staticcheck` into CI, and no fix for the installed `staticcheck` 2025.1.1, which cannot read the Go 1.27 standard library today for reasons unrelated to this change.
- No product code, public API or dependency change.

## Decisions

**Directive at `go 1.26`.** The candidates were 1.24 (status quo), 1.25 (clears `pdfcpu` v0.12), 1.26 (clears v0.16.4) and 1.27 (the installed stable). 1.26 is the maximum of what the tools require and the minimum above nothing: `pdfcpu` v0.16.4 declares `go 1.26.0`, `golangci-lint` v2.15.0 needs 1.26 to build, and 1.27 would cut off consumers who are one release behind without unlocking anything. 1.25 was rejected because it would freeze the validator at v0.12–v0.13 and force the same decision again next release.

**No `toolchain` line.** Adding `toolchain go1.27.4` would make every build request that exact toolchain, so CI — which installs the directive's version — would download and use 1.27.4 anyway. That silently converts "prove the minimum works" into "prove this machine's version works", which is the property the current workflow comment protects. The directive alone keeps the floor honest while local 1.27.4 builds the module unchanged.

**Directive and validator in one commit.** `go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@v0.16.4` compiles the module with the selected toolchain, so under `go 1.24` it fails with a version error. There is no ordering of two separate changes in which both are green: bumping the pin first is red until the directive lands, and bumping the directive first leaves v0.11.0, which is green but pointless. One commit is the only shape that is green on arrival.

**`upload-artifact` straight from v4 to v7.** v5 and v6 buy only the Node 24 runtime, and v7 additionally moves the package to ESM; stopping at an intermediate major would leave another major bump to review later for no benefit. The step uses `name`, `path` and `if-no-files-found: ignore`, none of which changed; the new `archive` input stays unused because the artifact is a directory tree.

**Keep the manual-pin boundary where it is.** The `go` directive and the `pdfcpu` pin remain outside Dependabot's reach per the existing "Deliberate pins stay manual" requirement; this change is the human decision that requirement reserves. The `ubuntu-24.04` image and its poppler are deliberately untouched, which is also why no raster may be regenerated.

**Cleanup last, as its own commit.** `.cache/` contains `bin/pdfcpu` (the validator the tests use), `bin/golangci-2.15` and the module cache that `go install` builds from. Deleting it before verification removes the tools the verification needs; deleting it after is free. `dist/` is generated demo output the CLI reproduces (`go run ./cmd/pdfgen`) and `.cache/` is a build cache by definition. The four untracked files are not in git history, so their deletion is one-way — accepted deliberately: `certificate.html` is byte-identical to the tracked `testdata/browser/certificate.html`, `certificate.pdf` is generated output, the stock photo is re-downloadable, and `certificate-2.html` is the source template behind a fixture that stays tracked and self-contained.

## Risks / Trade-offs

- **A newer language version enables stricter vet or type-checker findings** → run `gofmt`, `go vet` and the full suite before pushing; the lint job is unchanged and catches what vet does not.
- **The strict validator may disagree with older output** → already measured: the corpus passes `pdfcpu` v0.16.4 in strict mode today. The pin stays manual, and the new "Coupled toolchain pins agree" requirement keeps the pair from drifting apart again.
- **Consumers on Go 1.24–1.25 can no longer build the module** → documented as **BREAKING** in the proposal and stated in the README next to the minimum. This is the cost of the change, not an accident of it.
- **`upload-artifact@v7` runs only when a test fails**, so a broken reference would surface on the first failing build rather than on the next green one → accepted: the inputs this step passes are unchanged, and the step is diagnostic, not a gate. The first failure after the merge proves it either way.
- **Deleting `.cache/` costs one cold build** → expected; nothing is lost but time, and the documented commands rebuild it.

## Migration Plan

One commit for the pins and the prose that documents them (`go.mod`, `ci.yml`, `README.md`), then verification, then a second commit for the working-tree cleanup. Rollback is a revert: each pin is a single line and no data or golden state is involved. The workflow keeps proving the declared minimum after the revert, because the directive moves with the pins it enables.

## Open Questions

None that can be deferred without changing the plan.
