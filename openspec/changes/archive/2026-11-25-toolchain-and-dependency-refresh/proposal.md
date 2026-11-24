# Proposal

## Why

The module's `go` directive is three releases behind the toolchain that builds it: `go.mod` declares `go 1.24` while the project is developed and tested on `go1.27.4`, the current stable release. Nothing in the source needs 1.24, so the floor buys compatibility nobody uses and costs freshness everywhere else.

That floor is also the only reason the independent validator is frozen: `ci.yml` installs `pdfcpu@v0.11.0` because `go 1.24` cannot build a newer release (`v0.12` needs Go 1.25, `v0.16.4` needs Go 1.26). The validator that gates every generated PDF is therefore five minors behind, and its pin has no owner — the coupling lives in a comment, not in a rule.

The same drift shows in the workflow's action references: `upload-artifact` is still `@v4`, three majors behind the current `v7`, while every other action is already current. Finally the working tree has stopped being a signal: four untracked experiment files (3.9 MB) sit next to 3.3 GB of local build and module caches, so a dirty `git status` no longer means anything.

## What Changes

- Raise the `go` directive from `1.24` to `1.26`: the minimum that builds the newest `pdfcpu` and still stays one release below current stable. **BREAKING** for consumers of the `pdfgen` package, who must now build with Go 1.26 or newer.
- Move the `pdfcpu` install pin from `v0.11.0` to `v0.16.4` in the same commit as the directive, because `go install` uses the toolchain the directive selects; a split change opens a window where CI cannot install its own validator.
- Move `upload-artifact` from `@v4` to `@v7`. The step's inputs (`name`, `path`, `if-no-files-found`) are unchanged; the major brings the Node 24 runtime, an ESM package and an optional `archive` input this workflow does not use.
- Restate the coupling where it is currently only prose: the comment next to the `go-version-file` input, the comment next to the `pdfcpu` install, and the README paragraph that explains which pins stay manual.
- Delete the untracked experiment files (`certificate.html`, `certificate-2.html`, `certificate.pdf`, `vincent-yap-…-unsplash.jpg`) and the regenerable local directories `dist/` and `.cache/` as the last step, after verification has run.
- Leave untouched: the `ubuntu-24.04` runner image and its poppler (they determine the golden rasters), the pinned `golangci-lint` v2.15.0, the action references that are already current, and the Dependabot configuration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the existing "Pinned, development-only toolchain" requirement asserts that `go.mod` is unchanged, which this change contradicts. Its intent — the linter is a development tool, never a module requirement, and no `go.sum` appears — is kept, while the directive becomes something a deliberate toolchain change may move. A second requirement makes the `pdfcpu`↔`go` coupling a rule instead of a comment: the pinned validator MUST be installable by the module's minimum Go.

## Impact

- **`go.mod`**: one line, the directive. Still zero requirements and still no `go.sum`.
- **CI**: `.github/workflows/ci.yml`: two version pins and three comments. No new job, no new secret, no change to the golden steps.
- **Documentation**: `README.md` states the Go minimum and the linter's own Go requirement in the same sentence; the sentence inverts once the minimum reaches 1.26 and must be rewritten rather than patched.
- **Golden data**: none. Neither poppler nor the runner image moves, so no raster is regenerated and every snapshot keeps its expected pixels.
- **Working tree**: 3.9 MB of untracked files and 3.3 GB of ignored caches disappear; `dist/` is generated demo output the CLI reproduces (`go run ./cmd/pdfgen`) and `.cache/` is a local build cache by definition.
- **Local tooling**: the installed `staticcheck` 2025.1.1 cannot read the Go 1.27 standard library it is pointed at today, independently of this change. The README does not document `staticcheck` and CI does not run it, so this stays out of scope and is recorded as a limitation, not fixed here.
- **Assumption**: `go 1.26` is preferable to `go 1.27` because it clears both coupled tools with the lower floor. Raised to 1.27, the directive would cut off consumers who are one release behind for no tool that requires it.
