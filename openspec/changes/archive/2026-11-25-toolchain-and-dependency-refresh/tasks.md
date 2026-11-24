# Tasks

## 1. Raise the toolchain floor and the validator it enables

- [x] 1.1 Set `go 1.24` → `go 1.26` in `go.mod`. Verify with `go mod tidy` followed by `git diff --stat go.mod`: the file keeps one `module` line, one `go 1.26` line, no requirement and no `go.sum`; then `go build ./...` succeeds.
- [x] 1.2 Move the install pin to `pdfcpu@v0.16.4` in `.github/workflows/ci.yml` and rewrite the two comments that explain it (the `go-version-file` one and the pin's own), so the pair is stated as one decision instead of two facts. Verify with `grep -n 'pdfcpu@v0.16.4' .github/workflows/ci.yml`, `grep -n '1\.24' .github/workflows/ci.yml` returning nothing, and by reading `go …` in the comment `go.mod` of `pdfcpu` v0.16.4 in the module cache to confirm 1.26 satisfies it.
- [x] 1.3 Rewrite the README sentences that state the minimum and the linter's own Go requirement, and the "pins stay manual" paragraph that names `pdfcpu` and the directive. Verify with `grep -rn 'go 1\.24\|Go 1\.24' README.md .github/ go.mod` returning nothing outside `openspec/changes/archive/`, and with the paragraph still naming the runner image and poppler as the pins that must not move.

## 2. Refresh the stale action reference

- [x] 2.1 Change `actions/upload-artifact@v4` to `@v7` in the snapshot-artifact step, leaving its `name`, `path` and `if-no-files-found` inputs as they are. Verify with `grep -o 'uses: [^ ]*' .github/workflows/ci.yml | sort -u`: each action appears with exactly one major, no reference mentions `upload-artifact@v4`, and no other step's inputs changed.

## 3. Integration gate with the new pair

- [x] 3.1 Run `gofmt -l $(git ls-files '*.go')` and confirm it prints nothing, since the language version change must not move formatting.
- [x] 3.2 Run the suite with the validator required and pinned to v0.16.4 (`PDF_REQUIRE_VALIDATOR=1 PDFCPU=<path to pdfcpu 0.16.4> go test ./...`) and confirm it passes with no "pdfcpu not available, skipping independent validation" line in the output.
- [x] 3.3 Run the same suite under `-race` and confirm it passes.
- [x] 3.4 Run `golangci-lint` v2.15.0 over `./...` with the pinned version and confirm zero findings, matching the pre-change baseline.

## 4. Clean the working tree

- [x] 4.1 Delete the four untracked experiment files (`certificate.html`, `certificate-2.html`, `certificate.pdf`, `vincent-yap-3dxgsYa3eEg-unsplash.jpg`). Verify with `git status --short` showing an empty output, since none of them is tracked and nothing in the tree references them.
- [x] 4.2 Delete `dist/` and `.cache/` as the last change. Verify with `git clean -ndX` printing nothing and `git status --short` still empty.
- [x] 4.3 Confirm the tree still builds from a cold cache after the deletion: `go build ./...` and `go vet ./...` succeed, and `.cache/` has not reappeared inside the repository.

## Workflow follow-up

- Archive the change after review; verify the archived `openspec/specs/code-quality/spec.md` carries "Coupled toolchain pins agree" and the rewritten "Pinned, development-only toolchain" scenarios.
- Remove the leftover `/tmp/gopdf-scratch` copy from planning if it is still present; it is outside the repository and not part of this change.
