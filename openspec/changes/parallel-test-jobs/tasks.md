# Tasks

## 1. Split the workflow

- [ ] 1.1 Create the `unit` job: checkout, setup-go, `pdfcpu`, `PATH`, vet, the reference-command import check, `go test ./...` with `PDF_REQUIRE_VALIDATOR=1`, and the external-module build — check: the job is green on its own
- [ ] 1.2 Create the `race` job: `pdfcpu`, `PATH`, `go test -race ./...` with the validator — check: the job is green and reports its own check
- [ ] 1.3 Create the `golden` job: poppler, `pdfcpu`, `PATH`, the explicit snapshot step with `PDF_REQUIRE_RENDER=1` and the `--- SKIP` grep, and the artifact upload on failure — check: the job fails when the rasterizer is removed and uploads the artifacts
- [ ] 1.4 Create the `fuzz` job with the five sessions and a timeout — check: the job is green and independent of the others
- [x] 1.5 Leave `lint` as it is, with its own check — check: `gofmt` and the pinned linter still run there
- [x] 1.6 Set per-job timeouts (`unit` 15, `race` 20, `golden` 20, `fuzz` 15, `gate` 5) and keep the pinned `ubuntu-24.04` for every job that renders — check: the workflow parses and every rendering job names the pinned image

## 2. The gate

- [ ] 2.1 Add the `gate` job with `needs` on all five, `if: always()` and the result loop — check: the gate fails when a dependency fails, is skipped or is cancelled
- [x] 2.2 Update branch protection to require the gate and record the name in `CONTRIBUTING.md` — check: the required check is the gate and no job named `test` is required
- [x] 2.3 Update the `CONTRIBUTING.md` CI section with the job map and the gate — check: the section matches the workflow

## 3. Proving the gates

- [ ] 3.1 Open a pull request and confirm every job reports its own check and the gate waits for all — check: five checks plus the gate, all green
- [ ] 3.2 Break a unit test on a scratch branch and confirm the gate fails and names the failing job — check: the gate is red, the pull request is blocked
- [ ] 3.3 Skip the `golden` job with a temporary condition and confirm the gate fails — check: a skipped job cannot pass the gate
- [ ] 3.4 Remove poppler from `golden` and confirm the render step fails rather than skipping — check: the `--- SKIP` grep or `PDF_REQUIRE_RENDER` fails the job

## 4. Measure and accept

- [ ] 4.1 Record the wall clock of a representative run before the split, from the recent runs on `main` — check: the number is in the pull request
- [ ] 4.2 Record the wall clock after the split and name the job on the critical path — check: the number is in the pull request and the critical path is one of `race`, `golden` or `fuzz`
- [ ] 4.3 Confirm the trigger configuration and the concurrency group are unchanged — check: one run per change, a new push cancels the previous run
- [ ] 4.4 `openspec validate parallel-test-jobs --strict` — check: valid
