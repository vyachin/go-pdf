# Design

## Context

The conversion pipeline is `html → css → profile → engine → layout → pdf`.
`internal/profile` sits between the CSS parser and the engine: it decides
whether a rule, declaration or attribute belongs to the closed profile. Today
the same package also implements the stylesheet collection stage
(`CollectStylesheets`, `CollectInlineStylesheets`, `loadStylesheet`,
`decodeSheet`, `charsetRule`, `resolveStylesheetPath`), so its import list
includes `internal/assets`, `internal/fsx`, `internal/asset`, `path/filepath`
and `internal/charset`. The engine constructs `profile.Options` with the
registry and the stylesheet limit, which shows the mixing: half of the struct is
validation policy and half is I/O policy.

`assets.Registry.Read` already resolves a URL reference (`asset.Resolve`), opens
the confined root, stats, reads, caches by identity and returns typed errors.
`resolveStylesheetPath` runs the same `asset.Resolve` plus its own
`filepath.Join`/`Abs` and then discards the path, because `loadStylesheet` passes
the raw href to `reg.Read`. That is the duplicate the change removes.

## Goals / Non-Goals

**Goals:**

- One package for loading and decoding stylesheets, one package for validation.
- One resolver: the asset registry.
- The import graph proves `profile` has no asset or filesystem dependency.
- Diagnostics, file positions, cascade order and PDF bytes unchanged.

**Non-Goals:**

- Changing how stylesheets are found in the document or their cascade order.
- Changing the encoding rules, the `@charset` policy or the path-confinement
  rules.
- Moving `@font-face` *path* resolution: it already goes through the asset
  registry in the engine's font stage; only the descriptor parsing stays in
  profile.

## Decisions

**New `internal/stylesheet` package rather than a loader interface in profile.**
The loader needs the registry, charset decoding and path mapping; an interface
would keep the asset types in profile's import graph and move the wiring rather
than the responsibility. A separate package has a small public surface
(`Collect`, `CollectInline`, `Options`) and can be tested with a real temporary
root, like the current tests. Alternative rejected: move the loader into
`internal/engine` — the engine would grow a charset/path module, and the loader
tests would need the whole engine.

**Keep inline collection with the loader.** `CollectInlineStylesheets` reads a
`<style>` element's text and needs no asset, but it shares parsing and decode
helpers with the linked path and is a collection step, not validation. Both
entry points live in the new package so the engine has one call site for
"collect what the document says". Alternative rejected: leave inline collection
in profile — it splits one traversal across two packages and keeps the loader's
decode helpers duplicated.

**Delete the pre-resolution instead of keeping it for error mapping.** The
registry returns `*assets.Error` wrapping `fsx.ErrOutsideRoot`, `ErrTooLarge`,
remote/data causes and open errors; the loader can map those to the same
`css/link-*` codes without a second `asset.Resolve` call. A pre-check that only
produces an error and is then repeated cannot be kept correct when the registry
changes. Alternative rejected: keep `resolveStylesheetPath` as a fast reject —
it is the duplicate this change exists to remove.

**Prove the boundary with a source-import test.** `internal/profile` will have a
test that parses its own production files with `go/parser` and asserts the
forbidden import prefixes are absent. `go list` would need the toolchain at test
time; `go/parser` is hermetic and points at the file that introduced the
dependency. Alternative rejected: a comment-only convention — the drift already
happened once.

## Risks / Trade-offs

- **A diagnostic code or position changes during the move** → the change keeps a
  table-driven matrix for missing, remote, `data:`, escaping and legacy-encoded
  sheets and asserts the code, message prefix and position for each. The PDF
  goldens must not move; any move is a review failure.
- **The new package duplicates `profile.Options` fields** → the loader options
  keep only the fields it uses (`BaseDir`, `Assets`, `MaxStylesheetBytes`,
  `Budget`), and validation keeps `Strict`/`Budget`; the two structs are
  intentionally different and are not mirrored.
- **Test churn hides a regression** → tests move file for file, not rewritten;
  the diff review checks that assertions are unchanged apart from package names.
- **`CONTRIBUTING.md` drifts** → the layering section is updated in the same
  change and its diagram is checked by review.

## Migration Plan

Internal only. The public `pdfgen` surface, diagnostics, option semantics and
output do not change. The release note can omit the change; the archive records
the new package and the deleted helper so a later reader knows where loading
went.
