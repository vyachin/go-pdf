# Tasks

## 1. Validator lookup (ENG-01)

- [x] 1.1 Add one lookup helper to each test package: `PDFCPU`, then `exec.LookPath`, then `/tmp/gobin/pdfcpu` — verification: tests pass with the local binary and with `PDFCPU` set
- [x] 1.2 Fail when `PDF_REQUIRE_VALIDATOR=1` and no validator is found; log the skip otherwise — verification: a run with the variable and an empty `PATH` fails, a normal run reports the skip

## 2. Continuous integration

- [x] 2.1 Add `.github/workflows/ci.yml` running `go vet ./...` and `go test ./...` with pdfcpu installed and `PDF_REQUIRE_VALIDATOR=1` — verification: workflow file parses and the commands match the local ones

## 3. Acceptance

- [x] 3.1 Run `go vet ./...`, `go test ./...` locally with and without the validator to confirm both paths — verification: strict mode fails without the tool, normal mode passes
- [x] 3.2 Run `openspec validate ci-validation` — verification: change is valid
