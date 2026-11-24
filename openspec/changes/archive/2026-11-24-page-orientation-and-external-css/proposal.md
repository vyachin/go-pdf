# Proposal

## Why

Real templates run into two things the engine lacks: a landscape page and CSS in a separate file. The very first run of four documents from the public html-to-pdf-tests set showed that three of the four are blocked by exactly this: `@page { size: landscape }` is not parsed, and `<link rel="stylesheet">` is rejected outright. Both capabilities are standard, cheap, and needed not by the tests but by the product.

The third document from the same set — a 35 by 11 table — revealed one more thing: the corpus has no single real large document. Twelve-row invoices verify neither page breaks nor header repetition on a long table.

## What Changes

- **Page orientation.** `@page { size }` SHALL accept `landscape` and `portrait`, both separately and together with a named size (`A4 landscape`). Orientation without a size name applies to the default size from the launch parameters.
- **External stylesheets.** `<link rel="stylesheet">` with a local path SHALL be loaded and SHALL participate in the cascade in document order. Remote URLs SHALL be rejected: the engine does not go to the network. `media` accepts `print` and `all`; other `rel` values are ignored.
- **External CSS diagnostics.** An error in an included file SHALL point to the file, line and column in that file, not in the HTML.
- **Scripts.** `<script>` SHALL be ignored with the warning `html/ignored-script`: the content is neither executed nor rendered. In strict mode (`-strict`) this is an error.
- **Strict mode.** The `-strict` flag on `validate` and `convert` turns profile warnings into errors.
- **"Big table" fixture.** An own 35×11 document for verifying pagination and `thead` repetition, without copying third-party files.

Not included: remote styles and fonts, `media` other than `print`/`all`, JavaScript execution, `size` with explicit dimensions plus orientation (the CSS grammar does not allow this).

## Capabilities

### New Capabilities

None: both capabilities fit into existing capabilities.

### Modified Capabilities

- `pagination`: the "Page size and margins" requirement is extended with orientation and a fallback size.
- `css-profile`: the "Supported elements" requirement gains `<link>` and `<script>` with caveats; requirements are added about local stylesheets and about ignoring scripts.

## Impact

- **Code**: `internal/css` (`size` grammar, rule source), `internal/profile` (loading `<link>`, script policy, strict mode), `internal/layout` (fallback size in `ParsePageStyle`), `internal/engine` (access to `BaseDir` for CSS, `Options.Strict`), `cmd/pdfgen` (the `-strict` flag).
- **Diagnostics**: rule positions stop being offsets inside the HTML — every rule carries its own source. This affects the existing position tests and formatting.
- **Tests**: new scenarios for orientation, stylesheet order, `media`, a missing file, a remote URL, the script policy and strict mode; a big-table fixture with golden text.
- **Documentation**: `docs/profile.md` (elements and at-rules), `README.md` (the `-strict` flag).
- **Licenses**: documents from html-to-pdf-tests are not copied — the repository has no license. The fixture is written from scratch based on the idea, upstream is used only as a reference.
- **Compatibility**: behavior changes for templates with `<script>` (previously an error, now a warning) and for `<link>` (previously an error, now loading). This is an extension, existing documents do not break.
