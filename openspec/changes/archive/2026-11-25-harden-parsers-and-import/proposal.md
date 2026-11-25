# Proposal

## Why

`MaxDOMDepth` is enforced in the token pass against an open-element stack that is not
the tree construction's stack, so the limit is wrong in both directions: 300 sibling
`<p>` with omitted end tags (a 3-deep tree) fail with `resource/limit`, while
`<p><div></p>` repeated builds a 23-deep tree that a limit of 8 accepts. The token
stack also never inserts the implied `tbody` and does not merge repeated
`html`/`body`/`head`/`main`. Alongside that, the tag-name intern table silently returns
0 after 65,535 names (corrupting depth tracking), x/net's "open stack exceeds 512
nodes" error escapes without a code, CSS hex escapes are not decoded (`"\2014"` renders
`2014`), an explicit legacy charset leaves a UTF-8 BOM visible, `border: transparent`
paints black while `border: … rgb(…)` is rejected, and the browser import subtracts an
unresolved `em` border and silently drops auto/negative flex margins.

## What Changes

- The nesting limit SHALL be enforced on the tree the standard algorithm builds, by an
  iterative depth walk that runs after `html.Parse` and before the conversion model or
  layout, and MUST reject with `resource/limit`.
- The token pass SHALL keep a sound early rejection for a run of start tags that can
  never be implicitly closed, and outside table or foreign content; ambiguous nesting
  MUST be decided on the tree.
- The intern table SHALL grow or fail explicitly; every x/net construction error MUST
  become a coded diagnostic with a position.
- CSS string escapes (hex and escaped newline) SHALL be decoded; selector splitting
  SHALL be token-aware.
- An explicit charset SHALL strip a UTF-8 BOM before decoding.
- `border` SHALL accept the colour forms `color` accepts; `transparent` SHALL paint
  nothing.
- The browser import SHALL resolve `em` borders before box-sizing subtraction and
  report dropped auto/negative margins.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `html-parsing`: nesting is measured on the standard tree with a sound early rule; the
  intern table and x/net errors are bounded and coded; a BOM is stripped under an
  explicit charset.
- `library-api`: an over-limit nesting depth is reported before the conversion model and
  layout allocate, not necessarily before the standard tree exists.
- `css-profile`: escapes are decoded and border colours match `color`.
- `browser-import`: box-sizing and margin lowering use resolved values and report what
  they drop.

## Impact

- **Code**: `internal/html/scan.go` (early depth rule), `parse.go` (iterative depth walk
  and typed error), `internal/engine/convert.go`, `internal/css/token.go`, `parse.go`,
  `props.go`, `internal/layout/paint.go`, `internal/browser/import.go`.
- **Output**: valid HTML with omitted end tags converts again; trees deeper than the
  limit are rejected; escaped content renders correctly; hidden layout differences are
  reported.
- **Corpus**: `testdata/mpdf/cases.json` is re-measured where a wrong depth rejection
  changes a document's classification.
- **Tests**: each reproduction becomes a regression, a differential depth oracle, and
  fuzz seeds.
