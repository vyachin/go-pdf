# Tasks

## 1. The remaining substitutions

- [x] 1.1 Use `strings.Cut` and `strings.CutSuffix` in the data URI parser — check: a table test covers the accepted forms and the four malformed ones (no comma, no `;base64`, empty payload, unknown media type)
- [x] 1.2 Use `errors.Is(err, fs.ErrNotExist)` in `fsx` — check: the existing missing-file tests and a wrapped-error test pass
- [x] 1.3 Rewrite the two engine benchmarks with `testing.B.Loop` — check: `go test ./internal/engine -bench . -benchtime 1x` runs

## 2. Test order

- [x] 2.1 Add `-shuffle=on` to the test and race steps — check: the workflow runs both steps with the flag
- [x] 2.2 Run the suite shuffled locally and fix every order dependence it reveals, without disabling the flag — check: three consecutive shuffled runs are green
- [x] 2.3 Record the flag in `CONTRIBUTING.md` — check: the documented local command matches CI

## 3. Acceptance

- [x] 3.1 Confirm no behavior change: the golden layers and the diagnostic tests are green without regeneration — check: `go test ./...` with `PDF_REQUIRE_VALIDATOR=1`
- [x] 3.2 `go vet ./...` and the pinned `golangci-lint` — check: green
- [x] 3.3 `openspec validate stdlib-sweep-remainder --strict` — check: valid
