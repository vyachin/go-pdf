# Proposal

## Why

The independent PDF validator is optional: both the engine test and the mPDF corpus look only at `/tmp/gobin/pdfcpu`, skip silently when it is absent, and log nothing in the corpus. That is exactly how a malformed font object graph (PDF-01) stayed green — the engine's own reader followed the structure it wrote, so nothing caught it.

## What Changes

- The validator SHALL be located with `exec.LookPath`, with the explicit `PDFCPU` variable as an override and `/tmp/gobin/pdfcpu` as the last local fallback.
- When `PDF_REQUIRE_VALIDATOR=1` is set and no validator is found, the tests SHALL fail instead of skipping; a skip SHALL be logged otherwise.
- The repository SHALL run `go vet` and `go test ./...` in CI with the validator installed and `PDF_REQUIRE_VALIDATOR=1`, so a missing tool fails the build.

Out of scope: an independent renderer (pixel check), which needs a second tool and stays a follow-up.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: adds a requirement that the test pipeline validates output with an independent tool when the environment requires it.

## Impact

- **Code**: `internal/engine/convert_test.go`, `internal/mpdfcompat/corpus_test.go`.
- **CI**: new `.github/workflows/ci.yml` installing pdfcpu and running vet plus tests in strict mode.
- **Behaviour**: none in the product; local runs keep working without the tool and say so.
