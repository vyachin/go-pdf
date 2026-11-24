# Tasks

## 1. Linter configuration

- [x] 1.1 Add `.golangci.yml` with `version: "2"`, `linters.default: standard` and no presets or exclusions. Verify with `golangci-lint config verify` and confirm `golangci-lint run ./... --max-same-issues=0 --max-issues-per-linter=0` loads the file and still reports the baseline 72 findings.
- [x] 1.2 Confirm the configuration contains no `exclusions`, `presets` or per-linter `enable`/`disable` entries, and that the standard set is the only source of checks; record the baseline counts per linter in the commit message.

## 2. errcheck: explicit discards

- [x] 2.1 Replace the 18 unchecked `fmt.Fprint`/`Fprintf`/`Fprintln` calls in `cmd/pdfgen/main.go` with explicit discards (`_, _ =`). Leave the checked `f.Close()` in `writeFileAtomic` alone. Verify `golangci-lint run ./cmd/pdfgen` reports no errcheck findings and `go test ./cmd/pdfgen` passes.
- [x] 2.2 Handle the 9 production resource findings: `internal/fsx/fsx.go` (2), `internal/profile/styles.go` (1), `internal/pdfdebug/extract.go` (1), `internal/layout/layout.go` (1, `stopReason` in `tick`), `internal/engine/convert.go` (1), `internal/engine/fonts.go` (1), and the error-path `f.Close()`/`os.Remove` in `cmd/pdfgen/main.go` (2). Use checked closes where the error can affect the result and explicit `_ =` elsewhere. Verify `golangci-lint run ./... --tests=false` reports no errcheck findings and the affected package tests pass.
- [x] 2.3 Handle the 22 test resource findings: `internal/fsx/fsx_test.go` (7), `internal/engine/convert_test.go` (6), `internal/layout/layout_test.go` (3), `internal/engine/render_independent_test.go` (3), `internal/layout/pagination_test.go` (2), `internal/pdf/writer_test.go` (1), using `t.Cleanup` or explicit `_ =`. Verify `golangci-lint run ./...` reports zero errcheck findings and `go test ./...` passes.

## 3. Remove dead code

- [x] 3.1 Delete declarations that have no users: `fillPolygon` in `internal/backend/raster/raster.go`, `parseInt` and `parseIdent` in `internal/css/value.go`, `attrInt` and the unused `y` field in `internal/layout/box.go`, `htmlNode` and its `node` field in `internal/layout/lines.go`, and `checkPageBreak` in `internal/layout/paginate.go`. Verify `go build ./...` succeeds and `go test ./internal/backend/... ./internal/css/... ./internal/layout/...` passes.
- [x] 3.2 Delete the unused `codes` helper in `internal/profile/profile_test.go` and the stale `//nolint:staticcheck` in `internal/layout/pagination_test.go`. Verify `go test ./internal/profile ./internal/layout` passes and both findings are gone.

## 4. Ineffectual assignments

- [x] 4.1 Resolve `internal/layout/layout_test.go`: `runRaw` appends stylesheet diagnostics to a `diags` value it never uses. Decide whether the harness must return the combined diagnostics or drop the append, implement the chosen behaviour, and verify the layout tests still pass (add a regression assertion if the diagnostics become observable).
- [x] 4.2 Resolve `internal/layout/lines.go`: `canWrap = false` after `flush` is never read. Determine whether it was meant to suppress a second break in the same iteration, then either remove it as dead or wire it into the condition. Verify the wrapping and preformatted-text tests pass and `ineffassign` reports nothing.

## 5. staticcheck findings

- [x] 5.1 Fix the 8 quickfix findings: `QF1002` in `internal/css/props.go` and `internal/css/selector.go`, `QF1001` in `internal/css/token.go`, `internal/layout/pagination_test.go` (2) and `internal/pdf/writer_test.go`, `QF1006` in `internal/layout/paginate.go`, `QF1012` in `internal/engine/convert_test.go`. Verify `golangci-lint run ./...` reports no `QF` findings and the affected tests pass.
- [x] 5.2 Fix `S1008` in `internal/layout/paginate.go` (return the negated condition). Verify no `S1008` finding remains and `go test ./internal/layout` passes.
- [x] 5.3 Replace `runtime.GOROOT()` with `go/build.Default.GOROOT` in `internal/engine/convert_test.go` (2 sites) and `internal/pdf/writer_test.go` (1 site), keeping the skip when the fixture is absent. Verify no `SA1019` findings remain and `go test ./internal/engine ./internal/pdf` passes.

## 6. Continuous integration

- [x] 6.1 Add a `lint` job to `.github/workflows/ci.yml` with the same `push`/`pull_request` triggers: checkout, `actions/setup-go@v5` from `go.mod` with `cache: false`, a `gofmt` guard (`gofmt -l .`, print files and diff, exit 1), and `golangci/golangci-lint-action@v9.3.0` with `version: v2.15.0`. Verify the file parses as YAML and the two shell steps pass locally.
- [x] 6.2 Confirm the `test` job is unchanged and the `lint` job installs neither poppler nor pdfcpu; verify `git diff .github/workflows/ci.yml` adds only the new job.
- [x] 6.3 Run the `lint` job's steps in a clean checkout (a temporary worktree or `git archive` extract) so ignored local directories cannot mask a problem. Verify the `gofmt` guard passes and `golangci-lint run ./...` reports zero findings there.

## 7. Documentation

- [x] 7.1 Add a "Formatting and linting" section to `README.md` next to "Testing": the pinned `golangci-lint` version and a note that the prebuilt binary is used because v2.15 requires Go 1.26+ to build, `gofmt -w $(git ls-files '*.go')`, and `golangci-lint run ./...`. Verify each command runs as written and produces the documented result.
- [x] 7.2 Mention in the README testing/CI prose that CI enforces formatting and linting in a separate job. Verify the described checks match `.github/workflows/ci.yml`.

## 8. Integration verification

- [x] 8.1 From a clean tree run `gofmt -l $(git ls-files '*.go')` (empty output), `golangci-lint run ./...` (zero issues), `go test ./...` and `go test -race ./...`; all must pass.
- [x] 8.2 Verify `go.mod` is unchanged, no `go.sum` exists, and `git status` contains only the intended files.
- [x] 8.3 Verify the no-suppression requirement: `grep` finds no new `//nolint` directives and the configuration has no exclusions; confirm `golangci-lint run ./...` is green without them.

## Workflow follow-up

- After merge, mark the `lint` job as a required check in the repository's branch protection.
- Archive this change once review requirements are satisfied.
