# Tasks

## 1. The reference command

- [x] 1.1 Rewrite `cmd/pdfgen/main.go` to one conversion path: `pdfgen [-o out] input.html`, no subcommands, `pdfgen.Options{File: input, BaseDir: dir, FontDir: dir, ImportBrowser: true}`, diagnostics to stderr in `Diagnostic.String()` form, exit codes 0/1/2, and a write only after `Convert` succeeded. Verify: `go run ./cmd/pdfgen testdata/invoice.html` writes `testdata/invoice.pdf`, and `go run ./cmd/pdfgen` without an argument exits 2.
- [x] 1.2 Replace `cmd/pdfgen/main_test.go` with tests for the documented behaviour: default output path, `-o`, unknown flag exits 2, missing input exits 2, invalid template exits 1 with empty stdout and no file created, an existing output file is byte-identical after a failed conversion, and a relative image beside the input is embedded. Verify: `go test ./cmd/pdfgen` passes and each test names one `reference-cli` scenario.
- [x] 1.3 Add the direct-import check to the test job in `.github/workflows/ci.yml`: fail when `go list -f '{{join .Imports "\n"}}' ./cmd/pdfgen` contains `/internal/`. Verify: the check passes on the new command, and a temporary `internal/engine` import makes it fail (revert the probe).
- [x] 1.4 Update the command's doc comment and `-h` output so they show the single form and state that the command is an example without a compatibility promise. Verify: `go run ./cmd/pdfgen -h` prints one usage line and the example statement.

## 2. Guarantees

- [x] 2.1 Add `TestConvertConcurrent` to `pdfgen/pdfgen_test.go`: several goroutines convert the same template and different templates, and every result must equal the sequential result byte for byte. Verify: `go test -race ./pdfgen -run TestConvertConcurrent` passes, and a deliberately shared per-call cache makes it fail.
- [x] 2.2 Add `TestConvertWritesNothing`: `t.Chdir(t.TempDir())`, snapshot the directory entries, convert a self-contained document with an absolute `FallbackFont`, snapshot again, require no change. Verify: the test passes, and a temporary `os.WriteFile` inside the package makes it fail.
- [x] 2.3 Add `FuzzConvert` to `pdfgen/fuzz_test.go` with seeds from `testdata/` and small `Limits`: every iteration must end in a result or an error, a returned PDF must start with `%PDF`, and the process must not panic. Verify: `go test ./pdfgen -run '^$' -fuzz '^FuzzConvert$' -fuzztime 20s` runs without a crash or a failing seed.
- [x] 2.4 Add `FuzzConvert` to the fuzz step of `.github/workflows/ci.yml` next to the existing font fuzzers. Verify: the CI run lists the target and a locally broken seed fails the step.
- [x] 2.5 Add `TestDefaultOptions` asserting that `Options{}` yields A4, a 20 mm margin and the fixed creation date for a profile document with a declared font. Verify: `go test ./pdfgen -run TestDefaultOptions` passes, and the assertions match the values stated in the package documentation.

## 3. Caller documentation

- [x] 3.1 Move the package comment to `pdfgen/doc.go` and rewrite it as a contract: what it converts and for whom, the guarantees (deterministic output, no disk writes, no panics, concurrent safety, bounded work), what it does not do (browser CSS, JavaScript, network assets), the zero-value defaults, and links to the profile, the reference command and the licence. Remove the claim that the command is a thin wrapper over the package. Verify: `go doc ./pdfgen` reads as one contract and contains no command-parity claim.
- [x] 3.2 Review every exported identifier comment in `pdfgen/` so it states the contract, the defaults and the error behaviour instead of restating the name; reword the `Strict` comment so it no longer refers to a build. Verify: reading `go doc -all ./pdfgen` shows a contract line for each symbol.
- [x] 3.3 Add `TestExportedDocs` using `go/parser` and `go/doc` to fail on any exported identifier or package without a doc comment. Verify: the test passes on the tree and fails when a comment is temporarily removed.
- [x] 3.4 Add `ExampleError` for handling an `*Error` and keep the example set at the three caller decisions (`Convert`, `Validate`, `*Error`). Verify: `go test ./pdfgen -run Example -v` compiles all three and runs `ExampleError` against its expected output.

## 4. Front page and contributor entry point

- [x] 4.1 Rewrite `README.md` in the required order: what it is, install and a short conversion example, the guarantees, a pointer to the supported CSS, how to run the reference command, the compatibility statement, status and licence. Remove the feature tour, the flag tables, testing, formatting and linting, performance figures and the milestone table. Verify: a read-through against `library-docs` — an install command and a complete example appear before any reference material, and no contributor instruction remains.
- [x] 4.2 Add `CONTRIBUTING.md` as the maintainer entry point, covering the architecture diagram, the test layers, `docs/golden.md`, `docs/mpdf-corpus.md`, the formatting and linting workflow moved out of the README, the performance measurement, and the release procedure. Verify: every maintainer workflow is reachable from this one page, and the README repeats none of it.
- [x] 4.3 Delete the unreferenced `examples/` directory and confirm nothing links to it. Verify: `git grep -n "examples/"` reports no reference outside `testdata/`.
- [x] 4.4 State the versioning policy in the README and check it against `library-docs`: diagnostic codes, `Options` fields and deterministic output are named stable; the reference command, `internal/` packages and profile breadth are named unversioned. Verify: each named surface appears in both the README statement and the `reference-cli` spec.

## 5. Adoption material

- [ ] 5.1 Add `LICENSE` with the licence text confirmed by the copyright holder (MIT proposed in the design's open question) and link it from the README. Verify: the file exists, the README links it, and the holder and year are correct.
- [ ] 5.2 Add `CHANGELOG.md` with an `Unreleased` section and a `0.1.0` entry that summarises the milestones reached before the tag and records the removal of the old command surface. Verify: the entry matches the tag and the removed flags and subcommands are listed.
- [ ] 5.3 Tag `v0.1.0` on a green commit and verify consumption from a scratch module: `go mod init tmp && go get github.com/vyachin/go-pdf/pdfgen@v0.1.0`, then a program that calls `Convert` builds and renders. Verify: the scratch program produces a PDF and `go doc` resolves the package.

## 6. Integration checks

- [ ] 6.1 Run `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` on the final tree. Verify: all three pass.
- [ ] 6.2 Run the golden layers with `PDF_REQUIRE_RENDER=1` and confirm no snapshot moved. Verify: `git status --porcelain testdata/golden` prints nothing.
- [x] 6.3 Run `gofmt -l $(git ls-files '*.go')` and `golangci-lint run ./...` under the pinned version. Verify: both are clean.
- [x] 6.4 Compile and run the README example as written in a scratch module against the checkout (a `replace` directive is acceptable). Verify: the snippet produces a PDF without edits.

## Workflow follow-up

- Archive the change once the project's review requirements are satisfied.
- Verify the archived specs contain `library-api`, `library-docs`, `reference-cli` and the three modified requirements.
