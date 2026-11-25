# Design

## Context

`go-core-first` removed the large private substitutes and the sort helpers. What is left is small and local: a `data:` URI split with `IndexByte`/`TrimSuffix` in the image loader, an `os.IsNotExist` branch in `fsx`, two benchmarks in the old `for i := 0; i < b.N; i++` form, and a test job that never varies test order.

## Goals / Non-Goals

**Goals:**

- Remove the remaining hand-written equivalents of standard-library calls.
- Make test order a checked property rather than an assumption.
- No behavior change and no new dependency.

**Non-Goals:**

- `t.Context()` in existing tests: churn without a defect.
- `encoding/xml` for `docdump`: the snapshot needs canonical formatting and attribute order.
- A lazy cache in `internal/charset`: it is keyed by encoding and built once with a `sync.Map`; `sync.OnceValue` is single-shot and does not fit a keyed cache.

## Decisions

### 1. `strings.Cut` in the data URI parser

`strings.Cut(src, ",")` replaces `IndexByte` plus slicing, `strings.CutSuffix(header, ";base64")` replaces `HasSuffix` plus `TrimSuffix`, and `strings.Cut(mime, ";")` replaces the second `IndexByte`. The semantics are identical for every input the profile accepts: the first comma separates the header from the payload, and a parameter list ends at the first semicolon. A test table pins the malformed forms — no comma, no `;base64`, an empty payload — so the swap cannot change an error.

### 2. `errors.Is(err, fs.ErrNotExist)` in `fsx`

`os.IsNotExist` unwraps only the errors the `os` package documents; `errors.Is` also matches a wrapped `fs.ErrNotExist`, which is what `os.Root` and future wrapping return. The observable behavior is unchanged for the current cases, and the boundary keeps reporting real causes rather than converting them into `ErrOutsideRoot`.

### 3. `testing.B.Loop`

`B.Loop` (Go 1.24) replaces the `b.N` loop: it keeps the body from being optimised away without `b.N` bookkeeping and it is the form the standard library documents. The project's `go` directive is 1.26, so no build tag is needed. The benchmark bodies themselves do not change.

### 4. `-shuffle=on` in the test job

The test and race steps add `-shuffle=on`. This is a check, not a guarantee: if a test depends on the order, the job fails and the test is fixed in this change. Skipping the shuffle on failure would hide exactly what the flag is for. `CONTRIBUTING.md` records the flag so a local run matches CI.

### 5. No behavior change

Every item is an equivalence except the shuffle, which only changes execution order. The golden layers and the diagnostic tests must stay byte identical, and that is the acceptance criterion.

## Risks / Trade-offs

- **A hidden order dependence surfaces** → fix the test; the flag is worth the one-time cost.
- **`B.Loop` changes benchmark numbers slightly** → acceptable; the benchmarks are not gates.
- **`Cut` returns a different result on a malformed input** → the table test enumerates the malformed forms the loader rejects today.
- **`errors.Is` matches more errors than `os.IsNotExist`** → intended; the caller still distinguishes `ErrOutsideRoot` and `ErrTooLarge` first.

## Migration Plan

None: an internal refactor and a CI flag. Reverting restores the previous calls.
