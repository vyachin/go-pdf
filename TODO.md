# TODO from the code review

Open work is tracked as OpenSpec changes under `openspec/changes/`. A gap that is
not an active change is listed in the "Known gaps" section of
`docs/profile.md`. This file records the review passes and the permanent fixtures
they left behind; it is the evidence list, not the task list. The adversarial corpus
lives in `testdata/adversarial/` and is registered in
`internal/engine/adversarial_test.go`. It runs in the unit and race jobs, and its
manifest test fails when a listed fixture is missing or an HTML fixture is not
registered.

## Fourth pass — November 25, 2026

Reviewed snapshot `7c7ac70` (v1.0.0 plus 86 commits): the public API, the engine
pipeline, CSS, layout, fonts, the PDF writer, the HTML front end and asset loading,
and every document under `docs/`. `go vet ./...`, `go test -shuffle=on ./...`,
`go test -race -shuffle=on ./...` and `golangci-lint run ./...` pass at that
snapshot, and a 45 s `FuzzConvert` plus a 30 s `FuzzParse` run found no panic. Every
finding below is outside the current suite's coverage, which is why the suite stays
green. Severity: **P1** is a crash, an out-of-memory path or a broken contract;
**P2** is wrong output or a limit bypass; **P3** is quality, documentation or
compatibility.

The pass records 8 P1, 22 P2 and 36 P3 items. RV4-03 to RV4-07 are one systemic
defect — the work budget does not cover the stages whose cost grows with the input —
but each has its own fix and its own test, so each is a separate row. "Probe" means
the case was reproduced against the public API or the package under test; the
scratch harnesses live in `.cache/review/` and are not part of the repository. Each
confirmed hostile case should leave a fixture under `testdata/adversarial/`,
registered in `internal/engine/adversarial_test.go` as described above; a
correctness case leaves a regression test in the package that owns the bug.

The pass is recorded as the OpenSpec change `fourth-review-hardening`. Every RV4 and
IMP id below appears in that change as a requirement or scenario title, with at
least one scenario; the change also carries the proposal, the design and the tasks,
so a fix or a standing rule can be reviewed against a normative statement. The rows
below remain the evidence list and the reproduction record.

### P1 — crash, OOM or a broken contract

#### RV4-01 — a GPOS `PairPos` format 1 table expands pairs without a cap or a charge

[gpos.go:244](internal/font/gpos.go#L244) inserts every pair of every PairSet into a
per-lookup map but never bounds `len(l.pairs)` and never charges
`budget.Guard`. Coverage format 2 is charged once per range rather than per glyph
([gpos.go:404](internal/font/gpos.go#L404)), so a 65,536-glyph coverage costs two
units. [fonts.go:143](internal/engine/fonts.go#L143) charges font parsing by file
size only, which a small file cannot cross.

Probe: an 81 KB font with coverage 256 and 20,000 pairs per set makes 5.12 million
inserts in 340 ms with 148 MB allocated while the guard records 259 of its
20,000,000 units. The 264 KB `bomb.ttf` through `pdfgen.Convert` takes 3.15 s and
about 2.4 GB of allocations with `err == nil`; `MaxOperations: 1000` still spends
3.15 s inside the font before reporting `resource/limit`. The format maximum,
16,380 covered glyphs times 65,535 pairs, is about 1.07 billion inserts from a
~330 KB font.

The corpus's `p1-crafted-gpos.html` charges lookup, subtable and coverage scans, not
pair expansion, so it stays green and does not cover this case.

Required outcome: a per-font cap on declared pairs (the shape of `maxKerningGlyphs`),
one `Guard.Add` per PairSet before its inner loop, and coverage charged by
expansion. A fixture with a small `MaxOperations` must stop with `resource/limit`
before the pair map is built.

#### RV4-02 — a stylesheet of stray braces becomes a million diagnostics and an OOM

[parse.go:256](internal/css/parse.go#L256) appends a `Problem` for every stray `}`;
[token.go:117](internal/css/token.go#L117) takes no `Guard`; and
[styles.go:80](internal/profile/styles.go#L80) charges only `len(src)/4`, so 1 MB
of stylesheet costs 0.26 million of the 20 million default units.
[profile.go:155](internal/profile/profile.go#L155) then turns every problem into a
`Diagnostic`, and both entry points return the whole list plus a joined error
string.

Probe on `Validate` with `<style>` followed by N `}`:

| Input | Diagnostics | Allocated | Error text | Time |
|---|---|---|---|---|
| 64 KiB | 65,536 | 87 MB | 3.4 MB | 41 ms |
| 256 KiB | 262,144 | 347 MB | 13.8 MB | 92 ms |
| 1 MiB | 1,048,576 | 1.35 GB | 55.5 MB | 318 ms |

That is about 700 MB of RSS per MiB of stylesheet, while the budget reports under
2 million of 20 million units. At `MaxInputBytes` (8 MiB) it extrapolates to
4–6 GB, which is an OOM kill rather than a `resource/limit`. The corpus's
`p1-nested-media.html` bounds at-rule nesting, not the number of reported problems,
so it does not cover this case.

Required outcome: thread the `Guard` through `Tokenize` and the parser, charge
tokens, and cap the stored problems per sheet (for example the first 100 plus one
`resource/limit`, then stop parsing).

#### RV4-03 — `measure` re-lays out subtrees at every nesting level, uncharged

[paginate.go:581](internal/layout/paginate.go#L581) calls `buildLines` from
`measure` unconditionally, ignoring `b.built`, and `measure` is reached per nesting
level from the `break-inside: avoid` path at
[paginate.go:290](internal/layout/paginate.go#L290) and
[paginate.go:428](internal/layout/paginate.go#L428). Every level re-splits and
re-shapes all descendant words, so cost is O(depth² × words).

Probe (A4, default limits): 240 nested `break-inside: avoid` divs, 300 words in
each, 369 KB input — 34.7 s and 626 KB of PDF. 60, 120 and 240 levels take 2.3, 8.9
and 34.7 s; the same text without nesting takes well under a second. With
`MaxOperations: 100000` the conversion still spends 21.9 s before the limit fires,
because the re-layout is not charged. The corpus's `p2-rules-elements.html` charges
cascade matching, not this repeated layout.

Required outcome: cache the built lines per box and width (a width-keyed `built`
field), charge every collected piece in `buildLines`, and add a regression fixture
whose `MaxOperations` stops it in milliseconds.

#### RV4-04 — shaping every rune is not charged to the budget

[shape.go:47](internal/text/shape.go#L47) builds `[]rune` from the normalized string,
a per-rune glyph slice and a per-rune state slice, and nothing in `text.Shape` or
`buildLines` charges the guard for the work or the memory.

Probe: a 2 MB single word with `MaxOperations: 100000` converts in 765 ms with
394 MB of allocations (about 200 bytes per input byte) and no error; the layout
stage charges four units for that document. At `MaxInputBytes` (8 MiB) this
extrapolates to more than 1.5 GB with fewer than a thousand units spent.

Required outcome: charge `len(runes)` in shaping and line building, and replace the
`states` slice with the two anchors actually used (`lastBase`, `lastMark`).

#### RV4-05 — CFF to TrueType conversion ignores the guard and the context

[font.go:96](internal/font/font.go#L96) calls `convertCFF` without
`ParseOptions.Guard`; [cff_convert.go:50](internal/font/cff_convert.go#L50) converts
every `maxp` glyph, and [cff_charstring.go:113](internal/font/cff_charstring.go#L113)
caps instructions per glyph only, so the font-wide bound is glyphs × 100,000.
Cancellation is observed only after the whole conversion.

Probe: a 459 KB OTTO font whose 65,535 charstrings each call a ten-deep subroutine
chain takes 11.0 s in `Load`, records zero guard units, and a cancelled context is
reported only after 11.2 s. The public `Convert` takes 11.5 s even with
`MaxOperations: 1000`.

Required outcome: pass the guard into `convertCFF`, charge per glyph and per
instruction or point, check `guard.Err()` between glyphs, and add a font-wide
instruction cap.

#### RV4-06 — PDF serialisation is charged per page, not per byte

[writer.go:222](internal/pdf/writer.go#L222) charges `len(p.ops)+1` per page three
times, one unit per font and one per object; font subsetting, `/W` and `bfchar`
construction, PNG pixel decoding and zlib compression are O(bytes) but O(1) in
units, and `assemble` is uncharged.

Probe: a one-page document with 9,000 CIDs writes 1,561,760 bytes while recording
18 units; through the public API a 27 KB CJK input produces a 3.86 MB PDF with about
30 units. `MaxOperations` therefore cannot bound writer cost; only the font and
image limits can.

Required outcome: charge serialized bytes in `assemble` (for example one unit per
4 KiB), per serialized glyph and CID, and per decoded image pixel.

#### RV4-07 — adjacent-sibling matching scans siblings linearly and is not charged

[selector.go:258](internal/css/selector.go#L258) finds the previous element by
walking `Parent.Children`, and [cascade.go:118](internal/css/cascade.go#L118) charges
one unit per compound of a rule, not per sibling visited.

Probe with 100,000 siblings: `p p` takes 121 ms, `p + p` takes 4.7 s, both recording
5,300,106 units. 20,000, 40,000 and 80,000 siblings with `p + p` take 217 ms, 747 ms
and 4.4 s (versus 84, 128 and 250 ms for `p p`). Extrapolated to budget exhaustion
at about 370,000 elements, roughly two minutes of CPU precede `resource/limit`.

Required outcome: store a child index or previous-element pointer on `html.Node` so
the hop is O(1), or charge per visited sibling. A fixture with many siblings and a
small `MaxOperations` must stop early.

#### RV4-08 — the next release must be v2.0.0, and the package does not say so

Tag `v1.0.0` carries `Options.ImportBrowser` (`git show v1.0.0:pdfgen/pdfgen.go`);
the field is gone at HEAD and [CHANGELOG.md:9](CHANGELOG.md#L9) records the removal
under `[Unreleased]`. [README.md:164](README.md#L164) and
[CONTRIBUTING.md:223](CONTRIBUTING.md#L223) promise that within 1.x no release
removes what exists and that a breaking change takes a major release. Releasing the
Unreleased section as 1.x breaks both semver and the published promise, and the
README describes HEAD's API while `go get` returns v1.0.0.

Required outcome: publish the removal as v2.0.0 with a migration section (or keep a
deprecated field in a v1 line), and state in the changelog and the README which
version the documented API belongs to.

### P2 — wrong output or a limit bypass

| ID | Finding | Where | Evidence | Required outcome |
|---|---|---|---|---|
| RV4-09 | `Validate` and `Convert` report different problems: metadata is checked only by `Convert`, and the PDF-writing budget is charged only there. | [engine.go:165](internal/engine/engine.go#L165), [convert.go:313](internal/engine/convert.go#L313) | `Title: "bad \xff"` gives `options/title` from `Convert` and `nil` from `Validate`. `MaxOperations: 4140` makes `Validate` succeed and `Convert` fail with `resource/limit`. README:72 and the `Validate` godoc promise the same diagnostics. | Move the metadata checks into `prepare`; document and/or charge serialisation so a CI gate built on `Validate` cannot pass what `Convert` rejects. |
| RV4-10 | A `<style>` element inside a `<table>` takes another node's text as its content, so the wrong CSS is applied silently. | [parse.go:226](internal/html/parse.go#L226), [parse.go:254](internal/html/parse.go#L254) | `<table><style>p{color:red}</style>p{color:blue}<tr><td>y</td></tr></table>` parses the rule as `p{color:blue}` with no diagnostic; element positions are swapped too. | Record token offsets during tree construction, or at least verify a sliced span against the node's text and fall back to `n.Data`. Regression test for foster-parented style, script and text. |
| RV4-11 | `font-family: inherit` is accepted as a literal family name, then falls through to the system fallback. | [props.go:638](internal/css/props.go#L638), [fonts.go:244](internal/engine/fonts.go#L244) | With `@font-face` family "Print" and `body{font-family:"Print"}`, `p{font-family:inherit}` renders `/BaseFont /…+Arial` while `p{font-family:"Print"}` renders `…+DejaVu-Sans-Book`; no diagnostic. The other properties probed (`color`, `margin`, `text-align`, `display`) reject `inherit`. | Reject CSS-wide keywords in `font-family` (like the other properties) or implement `inherit` by cloning the parent list. |
| RV4-12 | `break-inside: avoid` on a box taller than the page creates an empty first page. | [paginate.go:289](internal/layout/paginate.go#L289) versus [paginate.go:427](internal/layout/paginate.go#L427) | 60 `line<br>` inside `break-inside:avoid` produce three pages; page 1 has no text. `placeChild` guards the same decision with `h <= contentHeight()`, `placeBlock` does not, and its `newPage()` return is ignored. | Require `h <= f.contentHeight()` before the page transition and honour a false return. |
| RV4-13 | A `rowspan` cell crossing a page break is painted once with the height of all covered rows and never continued. | [table.go:346](internal/layout/table.go#L346), [table.go:367](internal/layout/table.go#L367) | `rowspan=60` gives the cell a height of 1,204,200 mp on an 841,890 mp page; the baseline lands at 1,255,012 and neither the text nor a continuation fill appears on page 2. | Clamp the painted height to the page bottom and paint the continuation fragment on the next page, or stop painting when the fragment is entirely outside the page. |
| RV4-14 | A declared `height` is reserved in full on every fragment of a split box. | [paginate.go:599](internal/layout/paginate.go#L599) | `height:400pt` plus 54 lines paints the 400 pt background twice (1,128 pt in total) and pushes the following paragraph 342 pt down. | Track the offset already consumed by earlier fragments, or apply the declared height only to the first fragment. |
| RV4-15 | Table columns are not clamped to the available width, unlike block boxes, so later cells leave the page. | [table.go:22](internal/layout/table.go#L22) versus [paginate.go:241](internal/layout/paginate.go#L241) | `<td style="width:1000pt">WIDE</td><td>x</td>` puts `x` outside the page with no diagnostic; the same width on a `<div>` is clamped with a `layout/overflow` warning. | Clamp or scale the column sum to the available width and emit one `layout/overflow`. |
| RV4-16 | A long unbreakable token is painted past the content edge and clipped by the page without a diagnostic. | [lines.go](internal/layout/lines.go), [paint.go](internal/layout/paint.go) | 1000 `ж` in one word produce one page; the word's bbox ends at 597.3 pt on a 595.3 pt page and only about 50 characters are extractable, with no diagnostic. A 300-character CJK paragraph behaves the same. | Emit `layout/overflow` when the first word cannot fit, or break at grapheme boundaries; cover both the long-word and the CJK case. |
| RV4-17 | A document that declares windows-1252 is accepted as UTF-8 when its bytes are valid UTF-8. | [charset.go:84](internal/charset/charset.go#L84) | `<meta charset="windows-1252"><p>Привет</p>` validates cleanly, while `openspec/specs/html-parsing/spec.md:55` and [profile.md:49](docs/profile.md#L49) require `html/encoding`. The comment justifies only the pure-ASCII case. | Distinguish a declared windows-1252 from the no-declaration fallback (prescan the declaration) and reject the declared case, or amend the spec and the profile. |
| RV4-18 | `media` on `<style>` works but is reported as an unsupported attribute, and `Strict` turns it into an error. | [profile.go:121](internal/profile/profile.go#L121), [styles.go:72](internal/profile/styles.go#L72) | `<style media="print">` emits `html/unsupported-attribute`; with `Strict: true` the conversion fails, although [profile.md:212](docs/profile.md#L212) documents the feature and `CollectStylesheets` honours it. | Allow `media` on `style` and share one media-list parser between collection and validation. |
| RV4-19 | A pseudo-element in a non-final compound is accepted and applied to the following compound. | [selector.go:66](internal/css/selector.go#L66), [selector.go:284](internal/css/selector.go#L284), [profile.go:189](internal/profile/profile.go#L189) | `a::after span { color:#ff0000 }` validates and paints the `span` red (`1 0 0 rg` in the content stream). `a::after + p` behaves the same. | Reject a compound that carries a pseudo-element unless it is the last compound, in the parser and in profile validation. |
| RV4-20 | `height: 50%` validates but has no effect: layout resolves only absolute heights. | [props.go:53](internal/css/props.go#L53), [paginate.go:400](internal/layout/paginate.go#L400) | The declaration validates and renders like `height:auto`, with no diagnostic, although [profile.md:77](docs/profile.md#L77) documents `height` as a length or `auto`. | Reject a percentage for `height`, or resolve it against the containing block. |
| RV4-21 | The image format is chosen from the file extension rather than the content. | [images.go:126](internal/engine/images.go#L126) | A PNG named `misnamed.jpg` fails with "invalid JPEG format: missing SOI marker"; a PNG named `logo` fails with "unsupported image format"; the documented contract is PNG and JPEG by format. | Sniff the magic bytes (`http.DetectContentType` or a decode-config fallback) and keep the diagnostic about formats, not names. |
| RV4-22 | An asset reference is treated as a filesystem path: query, fragment, percent-encoding and surrounding spaces are not handled. | [images.go:86](internal/engine/images.go#L86), [styles.go:323](internal/profile/styles.go#L323) | `img src="logo.png?v=2"`, `logo.png#x`, `logo%20x.png` and `src=" logo.png "` all fail with `img/load` naming the polluted path; `link href="style.css?v=1"` fails the same way. | Resolve the reference as a URL: strip `?` and `#`, percent-decode the path, trim ASCII whitespace, and treat `//host` as remote. |
| RV4-23 | An invalid `/CreationDate` is produced when the UTC instant of a valid local date falls in year 10000. | [convert.go:72](internal/engine/convert.go#L72), [writer.go:738](internal/pdf/writer.go#L738) | `Date: time.Date(9999,12,31,23,30,0,0, time.FixedZone("m12", -12*3600))` writes `D:100000101113000Z`; `pdfcpu validate -m strict` rejects it as an invalid date. Only the local year is checked. | Validate `Date.UTC().Year()` in `prepare` and keep a defensive check in `pdfDate`. |
| RV4-24 | A 16-bit PNG is quantised to 8 bits and every PNG is written as 8-bit DeviceRGB. | [writer.go:670](internal/pdf/writer.go#L670) | `color.NRGBAModel.Convert` keeps 8 bits per channel, so a 16-bit grayscale source and its 8-bit twin produce identical image streams; a 1-bit grayscale image becomes 24-bit RGB. `openspec/specs/pdf-output/spec.md:69` says PNG is embedded "without losing pixel data". | Preserve the source colour type and bit depth (DeviceGray, 16-bit samples), falling back to RGB only for colour sources. |
| RV4-25 | The diagnostic catalogue in the profile document lists 19 of the 45 codes the code emits. | [profile.md:249](docs/profile.md#L249) | Missing include `img/load` (named by `openspec/specs/library-api/spec.md:183`), every `options/*`, `font/unavailable`, `pagination/unknown-target`, `css/link-*`, `pdf/write`, `layout/loop`, `layout/no-progress`, `page/margin-box`. Codes are the stated stable surface. | Complete the catalogue, ideally generated from a registry, and add a test that every emitted code is documented and every documented code is emitted. |
| RV4-26 | The README advertises browser lowering that has been removed. | [README.md:102](README.md#L102), [doc.go:30](pdfgen/doc.go#L30) | "a browser-oriented template can be lowered to it first" contradicts "there is no lowering pass" and the Unreleased removal of `Options.ImportBrowser`. | Say that a browser-oriented template must be rewritten within the profile. |
| RV4-27 | The profile document overclaims three behaviours. | [profile.md:142](docs/profile.md#L142), [profile.md:179](docs/profile.md#L179), [profile.md:243](docs/profile.md#L243) | `counter(page)` inside `a::after` and `target-counter` inside a margin box are rejected by [profile.go:238](internal/profile/profile.go#L238) and [profile.go:339](internal/profile/profile.go#L339); `options/*` and `resource/limit` diagnostics carry no position, contradicting "Every error contains the file, line, column". | Decide which side is authoritative for each case — the code allows strings and `target-counter` in `a::after`, and strings and page counters in margin boxes — and make the other side match; state the position rule precisely. |
| RV4-28 | An invalid option suppresses validation errors from a `<style>` element, contrary to the library-api scenario. | [convert.go:132](internal/engine/convert.go#L132) | `PageSize: "banana"` with `<style>p{display:flex}</style>` reports only `options/page-size`; the same declaration in a `style` attribute reports both. `openspec/specs/library-api/spec.md:201` requires both in one pass. | Validate inline rules without opening linked assets when an option is invalid, or narrow the scenario to declarations the pass can check. |
| RV4-29 | The documentation claims a pinned golden environment that CI does not use. | [ci.yml:78](.github/workflows/ci.yml#L78), [CONTRIBUTING.md:192](CONTRIBUTING.md#L192), [golden.md:128](docs/golden.md#L128) | Every job runs on `ubuntu-latest`, while the docs and `openspec/specs/code-quality/spec.md` require a pinned `ubuntu-24.04`; `testdata/golden/render/renderer.txt` records `pdftoppm 26.01.0` while [golden.md:78](docs/golden.md#L78) says 24.02.0 from Ubuntu 24.04. | Pin the image (or change the docs and the spec), and keep `renderer.txt` and the documentation in sync from one source. |
| RV4-30 | `pdfdebug` is not an independent parser, so structural writer regressions pass every test. | [extract.go:171](internal/pdfdebug/extract.go#L171), [extract.go:229](internal/pdfdebug/extract.go#L229), [extract.go:341](internal/pdfdebug/extract.go#L341) | A PDF with every `/Length` increased by 7 and one with every xref offset and `startxref` increased by 1 return the same text as the good file; a zeroed `/CIDToGIDMap` still extracts the text while the render shows notdef boxes; text outside the MediaBox is still counted. `pdfcpu` accepts the bad `/Length` too. | Validate `startxref`, xref entries and `/Length` in the extractor, and add a glyph-level or render-level check so an extractor cannot certify a broken file by itself. Document it as a text-layer oracle. |

### P3 — quality, documentation and compatibility

| ID | Finding | Where | Required outcome |
|---|---|---|---|
| RV4-31 | NBSP becomes U+0020 in the text layer, although U+00AD and U+2010 are preserved deliberately. | [normalize.go:109](internal/text/normalize.go#L109) | Keep U+00A0 in the run (the space glyph is the same); pin the extracted character in a test. |
| RV4-32 | Page geometry wraps on `int32` for extreme margins: `width` and `bottom` use plain subtraction, and line painting uses plain addition. | [paginate.go:51](internal/layout/paginate.go#L51), [paint.go:167](internal/layout/paint.go#L167), [lines.go:323](internal/layout/lines.go#L323) | Use `satAdd`/`satSub` and clamp the content box to a non-negative size; add a test with a margin larger than the page. |
| RV4-33 | On a 32-bit build, `cfg.Width*cfg.Height` is multiplied as `int` before the `int64` conversion, which can bypass `MaxImagePixels`. | [images.go:258](internal/engine/images.go#L258), [images.go:300](internal/engine/images.go#L300) | Multiply in `int64` before the comparison; cover with the existing 386 compile path or a unit test. |
| RV4-34 | A failed image load still consumes the aggregate byte budget, so a later good image can be rejected. | [images.go:113](internal/engine/images.go#L113) | Commit the total only after a successful decode, or refund on failure. |
| RV4-35 | A truncated JPEG is embedded as-is, while PNG is fully decoded for the same reason. | [images.go:309](internal/engine/images.go#L309) | Require an EOI or a full decode before embedding; keep the image budget charge. |
| RV4-36 | `fsx.Root.Read` resolves a relative path against the process working directory, not the root, and `Size` stats outside the root handle. | [fsx.go:123](internal/fsx/fsx.go#L123), [fsx.go:155](internal/fsx/fsx.go#L155) | Join relative paths with `r.dir` and stat through the root handle. |
| RV4-37 | `font/unavailable` carries no position although the `src` declaration has one. | [convert.go:177](internal/engine/convert.go#L177), [fonts.go:110](internal/engine/fonts.go#L110) | Carry the declaration position into the diagnostic. |
| RV4-38 | `firstMetaOffset` lowercases the window with `bytes.ToLower`, which changes byte length and shifts the reported column. | [charset.go:252](internal/charset/charset.go#L252) | Fold ASCII only, or search case-insensitively while preserving offsets. |
| RV4-39 | In a UTF-8 document, diagnostic columns count bytes: `<p>Привет</p><img src="missing.png">` reports 1:20 rather than the author's 1:14. | [node.go:276](internal/html/node.go#L276) | Decide the contract (byte or character column), pin it in a test, and align the README sentence about columns. |
| RV4-40 | A composite glyph without `ARGS_ARE_XY_VALUES` (0x0002) is placed as if its arguments were offsets. | [glyph.go:210](internal/font/glyph.go#L210) | Return an error like `sfnt` does, or implement point matching; add an outline comparison. |
| RV4-41 | The composite depth cap silently returns an empty outline, and the subsetter can drop the components of a used glyph. | [glyph.go:60](internal/font/glyph.go#L60), [subset.go:17](internal/font/subset.go#L17) | Return an error instead of an empty outline; the subsetter's visited set already stops cycles. |
| RV4-42 | GPOS lookups are applied without reading `ScriptList`, `FeatureList` or `lookupFlag`. | [gpos.go:98](internal/font/gpos.go#L98), [marks.go:38](internal/font/marks.go#L38) | Enable only the default-script `kern`, `mark` and `mkmk` lookups and honour `IgnoreMarks` and mark filtering sets. No corpus divergence observed yet. |
| RV4-43 | `fontRegistry.Face` builds its cache key with `%v` over the family list, so `["a b"]` and `["a","b"]` collide. | [fonts.go:219](internal/engine/fonts.go#L219) | Join with a delimiter a family name cannot contain, or key on the resolved face. |
| RV4-44 | System-fallback font parsing is not charged, unlike the `@font-face` path. | [fonts.go:405](internal/engine/fonts.go#L405) | Charge every load path. |
| RV4-45 | The sfnt oracle is narrower than CONTRIBUTING says: OTTO fonts are skipped and coloured or not-found glyphs are ignored. | [sfnt_oracle_test.go:95](internal/font/sfnt_oracle_test.go#L95), [sfnt_oracle_test.go:202](internal/font/sfnt_oracle_test.go#L202) | Narrow the claim to the TrueType parser, or feed converted fonts through the oracle and widen the sampled planes. |
| RV4-46 | Nested at-rules inside `@font-face` and margin boxes are dropped silently, while a nested `@page` is hoisted and applied. | [parse.go:310](internal/css/parse.go#L310), [parse.go:322](internal/css/parse.go#L322) | Report a problem for any nested at-rule the parent cannot represent and reject `@page` inside `@page`. |
| RV4-47 | A compound with two ids matches by the last one and counts one id of specificity. | [selector.go:405](internal/css/selector.go#L405), [selector.go:43](internal/css/selector.go#L43) | Reject a second id in a compound, or keep all ids and require every one to match and count. |
| RV4-48 | Backslash escapes are decoded in strings but not in identifiers, although the css-profile spec requires both. | [token.go:170](internal/css/token.go#L170), [token.go:316](internal/css/token.go#L316) | Decode escapes in ident and hash tokens (`caf\e9`, `.caf\e9`). |
| RV4-49 | `@media print, screen` is rejected while both collection paths implement media-list semantics. | [profile.go:200](internal/profile/profile.go#L200), [parse.go:82](internal/css/parse.go#L82) | Use one media-list parser in validation and collection. |
| RV4-50 | `Cascade.Compute` swallows `ApplyDecl` and `ParseContent` errors, and validation uses a fixed 12 pt parent, so validation and rendering can disagree. | [cascade.go:96](internal/css/cascade.go#L96), [profile.go:256](internal/profile/profile.go#L256) | Validate against the resolved parent size and never drop a declaration without a diagnostic. |
| RV4-51 | A `TJ` array grows to 24,000 elements and emits one string per glyph. | [writer.go:520](internal/pdf/writer.go#L520) | Pack contiguous glyphs into one hex string and split into operators of at most 8,191 elements; strings stay under 32,767 bytes. |
| RV4-52 | The `/W` array can hold thousands of widths in one group. | [writer.go:450](internal/pdf/writer.go#L450) | Split into multiple `startCID [widths]` groups. |
| RV4-53 | A zero-area image op is painted with a non-invertible matrix (seen in the tracked `p2-wide-image.html` fixture). | [writer.go:539](internal/pdf/writer.go#L539) | Skip image ops whose width or height is not positive. |
| RV4-54 | `/BaseFont` and `/FontName` can exceed the 127-byte architectural limit for a long family name. | [writer.go:324](internal/pdf/writer.go#L324), [writer.go:778](internal/pdf/writer.go#L778) | Cap the sanitized name so tag + name stays within 127 bytes. |
| RV4-55 | The budget comment says a cancelled context is checked under a mutex, but `Guard` has no mutex. | [budget.go:21](internal/budget/budget.go#L21) | Document `Guard` as single-goroutine, or add the mutex. |
| RV4-56 | Dead code and stale references: `ErrNotImplemented`, `profile.Text`, `profile.Elements`, `profile.Properties`, "used by the CLI" and "the importer". | [engine.go:158](internal/engine/engine.go#L158), [profile.go:355](internal/profile/profile.go#L355), [props.go:21](internal/css/props.go#L21), [cascade.go:32](internal/css/cascade.go#L32) | Remove what nothing calls, or wire it and document the caller. |
| RV4-57 | `PDFTEST_FONT` is read on the production path. | [fonts.go:367](internal/engine/fonts.go#L367) | Move the override behind an internal test seam or a documented option. |
| RV4-58 | The "find a test font" helper is copy-pasted into about ten packages. | `pdfgen`, `internal/{font,text,layout,pdf,engine,mpdfcompat}`, `cmd/pdfgen` | Move it into one internal test helper. |
| RV4-59 | `CharsetSupported("")` reports false although the empty `Options.Charset` is a valid request (detect). | [pdfgen.go:312](pdfgen/pdfgen.go#L312) | Report true for the empty name or document the guard callers must write. |
| RV4-60 | The `FontDir`, else `BaseDir` fallback is not in the godoc of `Options.FontDir`, and a `FontDir` that cannot be opened is reported with `options/asset-root`. | [pdfgen.go:24](pdfgen/pdfgen.go#L24), [fonts.go:62](internal/engine/fonts.go#L62) | Document the fallback and the diagnostic. |
| RV4-61 | `break-before: always` is accepted as an alias but is missing from the property table. | [props.go:702](internal/css/props.go#L702), [profile.md:96](docs/profile.md#L96) | List `always` next to `page`. |
| RV4-62 | After the budget is exhausted, later stages add misleading diagnostics such as `text/font`. | [fonts.go:229](internal/engine/fonts.go#L229) | Stop at the first budget failure; do not report a dependent missing font. |
| RV4-63 | Formatting defects: a list split by a paragraph, prose inside a `bash` fence, a reference to the removed `-page` flag. | [profile.md:203](docs/profile.md#L203), [mpdf-corpus.md:82](docs/mpdf-corpus.md#L82), [profile.md:153](docs/profile.md#L153) | Fix the list, close the fence, and name `Options.PageSize`. |
| RV4-64 | The open-work statement and the documented gaps disagree: only `archive` exists under `openspec/changes/`, while the docs list planned work. | [TODO.md:3](TODO.md#L3), [profile.md:193](docs/profile.md#L193), [mpdf-corpus.md:70](docs/mpdf-corpus.md#L70) | Open a change per documented gap, or move the gaps into a "known gaps" section with an owner. |
| RV4-65 | The benchmark figures in CONTRIBUTING are stale. | [CONTRIBUTING.md:171](CONTRIBUTING.md#L171) | Refresh them, or state that the numbers are indicative and give the command to measure. |
| RV4-66 | A PNG is decoded twice (once to validate in the loader, once for pixels in the writer) and the writer reads pixels through `At()` instead of the typed fast paths. | [images.go:272](internal/engine/images.go#L272), [writer.go:670](internal/pdf/writer.go#L670) | Keep the decoded image on `ImageData`, or decode once at write time, and add fast paths for `image.NRGBA`, `image.Gray` and `image.Paletted`. |

### Improvements for separate follow-up (fourth pass)

- **IMP-01 — a diagnostic code registry.** Put every code in one `internal/diag`
  file as a typed constant grouped by domain, generate the `docs/profile.md`
  catalogue from it, and add a test that every string literal passed to `Errorf`
  and `Warnf` is registered and every registered code is documented. This closes
  RV4-25 and prevents the next drift.
- **IMP-02 — one pipeline for `Validate` and `Convert`.** Run the same preparation,
  layout and metadata checks for both, with an output sink that either serialises or
  discards. This closes RV4-09 structurally instead of by a second code path.
- **IMP-03 — a cost model with invariant tests.** For every stage, document what one
  unit means and add a test of the form "for this adversarial fixture at
  `MaxOperations = N`, time and allocations stay within C × N". Cover RV4-01 and
  RV4-03 … RV4-07; the current `TestLimitsAreEnforced` checks only `MaxInputBytes`.
- **IMP-04 — positions from the tokenizer.** Record byte offsets while the tree is
  built instead of re-walking it, or verify every sliced span against the node's
  text. This closes RV4-10 and makes foster-parenting positions trustworthy.
- **IMP-05 — an asset pipeline that speaks URLs.** Sniff formats, strip query and
  fragment, percent-decode, trim, validate JPEG fully, and refund a failed load.
  Closes RV4-21, RV4-22, RV4-34 and RV4-35.
- **IMP-06 — one source of truth for documentation.** Derive the limits table from
  `engine.DefaultLimits()`, the code catalogue from IMP-01, and the CI image and
  renderer from the workflow and `renderer.txt`; add doc tests for each. Closes
  RV4-25, RV4-29 and prevents future drift.
- **IMP-07 — a release gate for breaking changes.** Publish the removal as v2.0.0
  with a migration section, and add a CI check that an `Unreleased` section marked
  breaking cannot be tagged as a minor. Closes RV4-08.
  *Withdrawn:* the v2.0.0 release and its migration section shipped, but the
  automated tag check was removed at the author's request; nothing enforces it
  now, and the requirement was withdrawn from the specs.
- **IMP-08 — independent PDF verification.** Teach `pdfdebug` to check `startxref`,
  xref entries and `/Length`, and add a glyph-level or render-level check on top of
  the text oracle; consider running `pdfcpu` in more than one mode in CI. Closes
  RV4-30 and strengthens the writer guarantee.
- **IMP-09 — CI hardening.** Pin the renderer image, add a coverage floor and make
  the independent validator mandatory for the writer tests. Closes RV4-29.
- **IMP-10 — test and utility hygiene.** One shared test-font helper, no environment
  hook on the production path, and deletion of the dead internal API. Closes
  RV4-56, RV4-57 and RV4-58.
- **IMP-11 — rendering performance.** Cache `measure` by width, key `FontFace` by a
  comparable struct, decode a PNG once, and use the typed-image fast paths instead
  of per-pixel `At()`. Covers RV4-03, RV4-43 and RV4-66.
- **IMP-12 — CSS grammar completeness.** One media-list parser, decoded ident
  escapes, an explicit policy for CSS-wide keywords, and a defined behaviour for
  nested at-rules. Covers RV4-11, RV4-46 … RV4-50.

Suggested order: the P1 rows first (RV4-01 … RV4-07), because each is reachable from
a small input and can exhaust a server; then RV4-08 with the release; then the
contract and documentation rows (RV4-09, RV4-25 … RV4-29); then the layout and CSS
correctness rows; then the P3 list and the improvement items.

### What this pass confirmed as correct

- The deterministic strategy — integer millipoints, fixed date, sorted diagnostics,
  a single binary-image identity — holds under 61 generated PDFs and concurrent
  conversions; no map iteration or clock leaks into the output.
- The adversarial corpus, the snapshot layers and the external validator are the
  right instruments; they only need the missing fixtures from this pass.

## Third pass — November 25, 2026

Reviewed the parser, CSS, font, image, layout and pagination limits and rechecked the
P1/P2 findings of the earlier passes. Every confirmed hostile case now has a permanent
fixture with a bounded outcome — a diagnostic code, one embedded object or a page
count — and none asserts elapsed time.

| Finding | Fixture | Expected outcome |
|---|---|---|
| P1 nested `@media` | `p1-nested-media.html` | `resource/limit`, also in a subprocess |
| P1 adjacent selector | `p1-adjacent-selector.html` | converts; the second paragraph is red |
| P1 many-spelling image | `p1-many-spellings.html` | one image object under `MaxOperations: 5000` |
| P1 crafted GPOS | `p1-crafted-gpos.html`, `gpos-bomb.ttf` | `resource/limit` under `MaxOperations: 250` |
| P2 wide image | `p2-wide-image.html`, `wide.png` | converts; no negative image scale |
| P2 DOM depth | `p2-dom-depth.html` | `resource/limit` at `MaxDOMDepth: 8` |
| P2 rules × elements | `p2-rules-elements.html` | `resource/limit` at `MaxOperations: 2000` |
| P2 repeated data URI | `p2-repeated-data-uri.html` | one image object |
| P2 page breaks | `p2-page-breaks.html` | two pages |
| P2 table grid | `p2-table-grid.html` | `resource/limit` at `MaxTableCells: 4` |

The reproductions that failed before each fix are recorded in the corresponding archived
changes under `openspec/changes/archive/2026-11-25-*`; the corpus itself was added by
`add-adversarial-regression-suite`.

## Second pass — November 24, 2026

Reviewed snapshot `12b26c4`: HTML/CSS parsers, fonts and text, layout and pagination, PDF writer, public API and CLI. **30 tasks closed: 5 P1, 18 P2, 7 P3** (RV2-01 … RV2-30). Every scenario was moved into a permanent regression test of the corresponding package, and the fixes went in as separate commits after `docs(todo): record the second review pass findings`.

## Previous pass — November 24, 2026

Reviewed snapshot `ef0e0d4`; **31 tasks closed: 2 P1 and 29 P2** (REV-01 … REV-34) plus three P3 proposals. All items were closed before the second pass; their evidence and acceptance records remain in the git history of `TODO.md`.

## History of earlier checks

Passes before November 24, 2026: 19 confirmed P2 bugs and improvement proposals, all closed. This section also preserved the checks' limitations (for example, no Poppler for independent text extraction and `staticcheck` unavailable for Go 1.27). Current status: `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` pass; PNG alpha and the resulting PDFs are checked by `pdfcpu` in strict mode.

## Improvements for separate follow-up (earlier passes)

Resource budgets (HTML/CSS/fonts/images, pages, table cells, operations) and
cancellation in all expensive stages are implemented. The fourth pass found stages that the budget does not cover (RV4-03 … RV4-07) and a release-policy gap (RV4-08), so this section is no longer empty; see IMP-03 above. The automated release check proposed as IMP-07 was withdrawn.

## Review addendum — 2026-11-24

The addendum's findings are closed: conversion cancellation, resource layout boundaries and the CI extension with an independent PDF check. There are no separate open items.

## Addendum: full review of the current project — November 24, 2026

P3 proposals are closed: shared geometry computation for blocks and cells (ARCH-01) and independent visual/text comparison of the resulting PDFs in CI (ENG-09). There are no open items.
