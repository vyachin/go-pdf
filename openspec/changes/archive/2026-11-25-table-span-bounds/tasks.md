# Tasks

## 1. Bounded spans (SEC-05)

- [x] 1.1 Validate `colspan`/`rowspan` in `buildTable` and report `html/invalid-span` for values outside `1..100` — verification: the overflowing `rowspan` reproduction fails with the diagnostic and does not panic
- [x] 1.2 Clamp every span read by layout before arithmetic and grid growth; divide in `rowHeight` only for `RowSpan > 1` — verification: a huge `colspan` is rejected, ordinary spans still lay out
- [x] 1.3 Cover the reproductions with tests — verification: overflow `rowspan`, huge `colspan`, and a normal `rowspan`/`colspan` table

## 2. Acceptance

- [x] 2.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate table-span-bounds` — verification: everything green
