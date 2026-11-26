# Tasks

## 1. Remove the unreferenced node helpers

- [ ] 1.1 Delete `InsertBefore`, `InsertAt`, `relinkFrom` and `ChildIndex` from `internal/html/node.go` and rewrite the foster-parenting comment so it describes the code that exists. Verify with `go build ./...`, `go vet ./internal/html` and `go test ./internal/html`; also check with `grep -rn 'InsertBefore\|InsertAt\|relinkFrom\|ChildIndex' --include='*.go' .` that no reference remains outside the removed definitions.
- [ ] 1.2 Add a package test that lists the remaining methods on `Node` that mutate children and asserts each has a caller or a test, so a newly added dead helper fails the test. Verify the test fails when one of the old helpers is restored and passes on the cleaned tree.

## 2. Make the property and token invariants executable

- [ ] 2.1 Add `TestPropOrderMatchesTable` in `internal/css`: compare `PropOrder` with the key set of `Table` and report a name that is present on one side only. Verify it fails on a temporary extra key in one collection and passes on the clean tree.
- [ ] 2.2 Correct the `Token.Text` comment to say that ident, hash and at-keyword tokens hold decoded text while strings and dimensions hold the source spelling; add an assertion in `TestTokenTextContract` (source-level, like `pdfgen`'s exported-docs test) that the comment still names those kinds. Verify with `go test ./internal/css`.

## 3. Complete and check the profile document

- [ ] 3.1 Edit `docs/profile.md`: add a `style` row with `media`, replace "contains print or all" with the whole-item rule (and the `only print` consequence), and add the percentage to the `line-height` row. Verify the text against `profile.allowedAttributes` and `css.MediaAppliesToPrint`.
- [ ] 3.2 Add `TestProfileTablesMatchTheCode` in `internal/profile/docs_test.go`: parse the element block, the attribute table and the inherited column, compare them with the allow-lists and `css.Table`, and fail with the drifted row name. Verify the test fails when one documented attribute or inheritance flag is changed and passes on the corrected document.
- [ ] 3.3 Keep the existing `TestProfileDocMatchesParser` green and run `go test ./internal/profile ./internal/css ./internal/html`.

## Workflow follow-up

- Run `openspec validate fix-dead-code-and-doc-drift --strict` and fix any reported issue.
- Sync both deltas into `openspec/specs/code-quality/spec.md` and `openspec/specs/library-docs/spec.md` and archive the change after review.
