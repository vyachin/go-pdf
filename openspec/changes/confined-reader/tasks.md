# Tasks

## 1. Confined reader package (CLEAN-02, SEC-04)

- [x] 1.1 Create `internal/fsx` with `Root`, `Open`, `Read`, `Close` and `ErrOutsideRoot`; resolve the root once — verification: unit tests for containment, internal symlink, escaping symlink
- [x] 1.2 Return the original error for a file that cannot be read inside the root — verification: a permissions test expects the access error, not `ErrOutsideRoot`

## 2. Rewire consumers (ENG-05)

- [x] 2.1 Make `profile.CollectStylesheets` open `BaseDir` once and read every sheet through it; drop `profile.ReadWithin` — verification: CSS boundary tests keep their codes
- [x] 2.2 Make the image loader open `BaseDir` once and reuse it — verification: image boundary tests pass; one root per conversion
- [x] 2.3 Make the font registry open its root once before iterating `@font-face` — verification: font boundary tests pass

## 3. Acceptance

- [x] 3.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate confined-reader` — verification: everything green
- [x] 3.2 Re-check the three review points: precise error, single root, package boundary — verification: unit tests demonstrate each
