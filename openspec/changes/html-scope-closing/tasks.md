# Tasks

## 1. Scoped implicit closing (HTML-01)

- [x] 1.1 Add a scoped close helper and use it for `li`, cells, rows, row groups, `dt`/`dd` and `option` — verification: the nested table reproduction keeps the inner row in the inner table
- [x] 1.2 Keep explicit end tags unscoped — verification: `</td>` and `</li>` still close the nearest element
- [x] 1.3 Assert structure and geometry — verification: tests check parent relationships for nested tables and lists, and that the trailing list text stays in the outer item
- [x] 1.4 Keep flat tables and lists unchanged — verification: existing table and list tests pass

## 2. Acceptance

- [x] 2.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate html-scope-closing` — verification: everything green
