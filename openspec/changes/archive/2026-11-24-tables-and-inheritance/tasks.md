# Tasks

## 1. Multi-row table header (LAY-02)

- [x] 1.1 Skip header rows only when they were actually repeated on the current page — verification: a two-row `thead` shows both rows on page one
- [x] 1.2 Repeat every header row once on continuation pages — verification: page two starts with both rows, not one

## 2. Rowspan-aware cell placement (LAY-01)

- [x] 2.1 Build an occupancy grid before measuring and store each cell's column on the box — verification: unit test on the review table checks that the second row's cell starts in column one
- [x] 2.2 Use the stored column in width computation, row height and painting — verification: reproduced positions match the expectation; `colspan` and `rowspan` combinations covered
- [x] 2.3 Keep plain tables unchanged — verification: existing table tests and the big table fixture pass

## 3. Inheritance through the DOM chain (CSS-02)

- [x] 3.1 Compute the `html` style and pass it as the parent of `body` — verification: `html { font-size: 30pt }` reaches body text
- [x] 3.2 Walk `table` → row group → `tr` → cell when computing cell styles — verification: `tr { color: red }` colours cell text
- [x] 3.3 Preserve attributes on the `html` element while parsing — verification: parser test keeps class, id and style; inline style on `html` inherits

## 4. Acceptance

- [x] 4.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate tables-and-inheritance` — verification: everything green
- [x] 4.2 Re-check the three reviewed reproductions by hand — verification: each produces the documented outcome
