# text-shaping Specification

## Purpose

Describes how characters of the input document become glyphs and lines: font loading, mapping code points to glyphs, kerning, line breaking, and the rules that Russian typography must not violate.

## Requirements

### Requirement: Font loading accepts local TrueType and OpenType fonts

The engine SHALL load fonts from local files named in `@font-face` or `Options.FallbackFont` in TrueType and OpenType formats. An OpenType font whose outlines are CFF SHALL be accepted through an in-memory conversion to TrueType. WOFF, WOFF2, CFF2, variable fonts and remote URLs MUST be rejected with a diagnostic naming the format or the source.

#### Scenario: A local TrueType font
- **WHEN** the template declares `@font-face` with a local PT Serif file in TrueType format
- **THEN** the font loads and is available for the document

#### Scenario: A local OpenType font with CFF outlines
- **WHEN** the template declares `@font-face` with a local `.otf` file whose outlines are CFF
- **THEN** the font loads, the text renders with its glyphs and the PDF embeds the glyphs the document uses

#### Scenario: A WOFF font
- **WHEN** the template declares `@font-face` with a local `.woff` or `.woff2` file
- **THEN** validation fails with a diagnostic naming the format

#### Scenario: A font by URL
- **WHEN** the template declares `@font-face` with `src: url(https://example.com/font.otf)`
- **THEN** validation fails: remote fonts are not supported

#### Scenario: A missing font file
- **WHEN** the path in `@font-face` points to a file that does not exist
- **THEN** a diagnostic reports the absolute path that could not be opened

### Requirement: CFF outlines convert within a tolerance, deterministically

A CFF glyph SHALL be converted to quadratic TrueType contours whose points stay within the tolerance recorded in `docs/fonts.md`, and the conversion MUST be deterministic: the same font bytes produce the same glyph data and the same PDF. The work and memory of a conversion SHALL be bounded; a font whose converted form exceeds the documented limit MUST fail with `resource/limit` instead of allocating without bound.

#### Scenario: Repeated conversion
- **WHEN** the same OpenType document is converted twice
- **THEN** the two PDFs are byte identical

#### Scenario: Outline fidelity
- **WHEN** a converted glyph is compared with the original CFF outline
- **THEN** every converted point lies within the recorded tolerance of the original curve

#### Scenario: The conversion limit
- **WHEN** a font converts to more than the documented limit
- **THEN** the conversion fails with `resource/limit` and no partial PDF is produced

### Requirement: Mapping code points to glyphs

The engine SHALL map characters to glyphs through a `cmap` table of format 4 or 12. If a character is missing from the selected font and from every alternative in the family, a diagnostic MUST name the character and its position in the document; silently substituting an empty glyph is not allowed.

#### Scenario: Cyrillic renders correctly
- **WHEN** the document contains the text "Счёт № 42 от 01.02.2026"
- **THEN** all characters, including "ё", "№" and "—", are rendered with existing glyphs

#### Scenario: Character missing from every candidate
- **WHEN** the text contains a character absent from the font family
- **THEN** rendering fails with a diagnostic naming the code point and its place in the document

### Requirement: Kerning from the GPOS table

The engine SHALL apply kerning pairs from the `GPOS` table when the font provides one. Without the table it MUST use widths from `hmtx` and report no error.

#### Scenario: A kerning pair is applied
- **WHEN** the font contains a kerning pair for adjacent characters and the document uses it
- **THEN** the distance between the glyphs differs from the sum of their `hmtx` widths

#### Scenario: Font without GPOS
- **WHEN** the font has no `GPOS` table
- **THEN** text is rendered with `hmtx` widths and no diagnostics

### Requirement: Malformed optional font tables do not crash parsing

Parsing a font with a truncated or inconsistent optional table SHALL NOT panic. Every field read MUST be preceded by a bounds check for that table format. A corrupt optional table MAY be ignored, in which case the font loads without the feature it provides; a corrupt required table MUST produce an error.

#### Scenario: Truncated class definition
- **WHEN** a class definition in a GPOS table ends before its range header
- **THEN** parsing does not panic
- **AND** the font either loads without kerning or returns an error

#### Scenario: Truncated kerning table
- **WHEN** the GPOS table is truncated in the middle of a pair set
- **THEN** parsing does not panic
- **AND** the font loads with no kerning pairs

#### Scenario: Required table missing
- **WHEN** a required table such as `glyf` is absent
- **THEN** parsing returns an error naming the missing table

### Requirement: Font parsing work is bounded

Font parsing SHALL charge the shared work guard for every lookup, subtable and coverage range it examines, and SHALL stop with a `resource/limit` diagnostic when the configured budget is exhausted. The number of entries a font declares MUST NOT let a small file bypass the budget, and parsing MUST observe context cancellation.

#### Scenario: The amplifier font stops at the budget
- **WHEN** a font whose GPOS table repeats one PairPos across 40,000 subtables is
  loaded with a work budget that the parse would exceed
- **THEN** the call returns `resource/limit` instead of parsing every subtable

#### Scenario: A normal font is unaffected
- **WHEN** a regular text font is parsed under the default limits
- **THEN** the font loads with its kerning pairs and no diagnostics

#### Scenario: Cancellation during parsing
- **WHEN** the context is cancelled while a large font is parsed
- **THEN** the call returns the context error

### Requirement: Font-declared sizes never overflow

Every offset, length and count read from a font SHALL be evaluated in 64-bit arithmetic before it is converted or used to slice, on every supported architecture. A value outside its table or the glyph-id range MUST be rejected, and a `cmap` format 12 group whose `startGlyphID + count` exceeds 0xFFFF MUST be rejected.

#### Scenario: A high offset on a 32-bit build
- **WHEN** a font declares a `cmap` offset of 0x80000000 on a 32-bit build
- **THEN** the font is rejected with a parse error and the process does not panic

#### Scenario: Format 12 glyph id overflow
- **WHEN** a format 12 group starts at `startGlyphID: 0xFFFFFFF0` and spans several
  characters
- **THEN** parsing rejects the group instead of mapping the characters to low glyphs

### Requirement: Non-breaking sequences are not split

The engine SHALL treat a no-break space (U+00A0) as a prohibition of a line break at that position and MUST keep it as a word separator when justifying.

#### Scenario: No-break space in an address
- **WHEN** the document contains "г." + U+00A0 + "Москва"
- **THEN** the pair stays on one line for every available break

#### Scenario: No-break space under justification
- **WHEN** a line containing a no-break space is justified
- **THEN** the gap at the no-break space is not stretched

### Requirement: Line breaking

The engine SHALL break lines at spaces and after a hyphen without splitting a word that has no hyphen, and MUST keep trailing punctuation with the preceding word. Hyphenation (`hyphens`) is not supported in v1 and MUST be rejected as an unsupported property.

#### Scenario: A long word wider than the line
- **WHEN** a word without spaces does not fit the available width
- **THEN** it moves to the next line as a whole instead of being split in the middle

#### Scenario: Requesting automatic hyphenation
- **WHEN** the template contains `hyphens: auto`
- **THEN** validation fails and states that hyphenation is outside the v1 profile

### Requirement: Justification

With `text-align: justify` the engine SHALL distribute extra space between word separators. The last line of a paragraph and the line before a forced break MUST remain unstretched.

#### Scenario: A paragraph of several lines
- **WHEN** a paragraph occupies three lines and is justified
- **THEN** the first two lines have the same width, equal to the available width, and the third stays unstretched

#### Scenario: A line before a page break
- **WHEN** a line ends the page and the next one starts a new page
- **THEN** the closing line is not stretched

### Requirement: Composition of combining characters

The engine SHALL normalise input text to canonical composition before mapping it to glyphs, so that "й" and "ё" written in decomposed form render as a single glyph rather than a base character plus a separate diacritic.

#### Scenario: Decomposed "й"
- **WHEN** the text contains "и" + U+0306
- **THEN** the output uses the "й" glyph from the font
- **AND** text extracted from the PDF contains a single "й" character

#### Scenario: Canonically composed text is unchanged
- **WHEN** the text is already written in canonical composition
- **THEN** mapping to glyphs produces the same result

### Requirement: Combining marks are positioned over their base

When normalisation leaves a combining mark as a separate code point, the engine SHALL
position it with the font's GPOS mark attachment (`MarkBase`, `MarkLig`, `MarkMark`)
when those lookups exist, so it is drawn over the base glyph rather than at the pen.
When the font cannot attach it, the engine MUST report a diagnostic instead of
silently drawing it at the wrong place.

#### Scenario: A stressed Russian word

- **WHEN** the text contains "сло́во" with U+0301 after "о"
- **THEN** the acute is drawn over "о"
- **AND** extraction still returns the combining sequence

#### Scenario: A font without mark attachment

- **WHEN** the font has no GPOS mark lookups for a separate mark
- **THEN** the document fails with a diagnostic naming the mark

### Requirement: Unicode separators break lines

Form feed (U+000C), line separator (U+2028) and paragraph separator (U+2029) SHALL
start a new line. A form feed MUST NOT reach glyph lookup, and no separator may join
two lines in the rendered output.

#### Scenario: A form feed in preformatted text

- **WHEN** `white-space: pre` text contains `A\fB`
- **THEN** two lines are rendered and no `text/glyph` error is reported

#### Scenario: A line separator

- **WHEN** text contains `A` U+2028 `B`
- **THEN** two lines are rendered

### Requirement: CFF values outside the TrueType range are rejected

A CFF charstring delta whose magnitude exceeds 32767 SHALL be rejected with a
diagnostic, never truncated to int16. The glyph's stored outline MUST stay consistent
with its header bounding box.

#### Scenario: An oversized delta

- **WHEN** a charstring draws a 32767.99998-unit line
- **THEN** loading the font fails with a diagnostic
- **AND** the glyph is not silently drawn with a 65,536-unit jump

### Requirement: Composite components apply scaled offsets

A composite glyph component whose flags include `SCALED_COMPONENT_OFFSET` (0x0800)
SHALL have its offset transformed by the component's scale before it is added.

#### Scenario: A scaled offset

- **WHEN** a component at offset (100, 0) with scale 0.5 sets the flag
- **THEN** the first point is the child's point plus 50, not plus 100

### Requirement: A CFF font without a Private DICT loads

A CFF font whose top DICT omits the `Private` operator SHALL load as if it declared an
empty private dictionary, with no local subroutines. It MUST NOT be rejected.

#### Scenario: A minimal CFF font

- **WHEN** an otherwise valid CFF font has no `Private` operator
- **THEN** the font loads and glyph 0 is drawn

### Requirement: GPOS pair expansion is capped and charged (RV4-01)

A `PairPos` subtable SHALL be bounded by a declared-pair cap and MUST charge the
work guard before it expands the pairs into a map. A font whose pair tables exceed
the cap MUST fail with `resource/limit` before the map is built, instead of
allocating a map entry per pair.

#### Scenario: A small font with a large pair table

- **WHEN** a 264 KB font declares a coverage of 1024 glyphs and a PairSet of 65535
  pairs per covered glyph
- **THEN** the conversion fails with `resource/limit` before allocating memory
  proportional to the expanded pair count
- **AND** the same font with a small pair table still converts

#### Scenario: The existing crafted-GPOS fixture stays green

- **WHEN** the corpus runs `p1-crafted-gpos.html` under `MaxOperations: 250`
- **THEN** it still reports `resource/limit`
- **AND** the new pair-table fixture is registered beside it

### Requirement: CFF outline conversion honours the guard and the context (RV4-05)

Converting CFF outlines to TrueType SHALL charge the work guard per glyph and per
instruction, MUST check the guard between glyphs, and MUST observe context
cancellation during the conversion. A font's whole conversion MUST be bounded by
the same budget as its parsing.

#### Scenario: A deep subroutine chain is bounded

- **WHEN** a 459 KB OTTO font has 65535 charstrings whose subroutines expand to
  hundreds of thousands of instructions
- **THEN** the conversion stops with `resource/limit` after work proportional to the
  budget
- **AND** cancelling the context during the conversion returns the context error
  promptly, not after the whole font is converted

### Requirement: A non-breaking space stays U+00A0 in the text layer (RV4-31)

Normalization SHALL keep a non-breaking space as U+00A0 in the glyph run and in the
`ToUnicode` map while still treating it as unbreakable, so that copied text carries
the character the author wrote.

#### Scenario: Copying a non-breaking space

- **WHEN** a document contains `a&nbsp;b`
- **THEN** the extracted text contains U+00A0 between the two letters
- **AND** the line is not broken at that space

### Requirement: A composite glyph without XY arguments is not placed as an offset (RV4-40)

When a composite component does not set `ARGS_ARE_XY_VALUES` (0x0002), the parser
MUST NOT treat its two arguments as an offset. It SHALL reject the glyph with a
diagnostic or implement point matching, and the result MUST be an error rather than
a silently wrong outline.

#### Scenario: A point-matching composite

- **WHEN** a font contains a composite glyph whose second component omits
  `ARGS_ARE_XY_VALUES` and supplies point indices
- **THEN** the glyph is rejected with a diagnostic or placed by point matching
- **AND** it is never placed at the point-index numbers as coordinates

### Requirement: A too-deep composite glyph is an error, not an empty outline (RV4-41)

A composite glyph beyond the depth limit SHALL be reported as unsupported. The
subsetter MUST keep every component of a glyph the document uses; it MUST NOT drop
the components because a depth cap returned an empty outline.

#### Scenario: A long composite chain

- **WHEN** a document uses a glyph that is a twelve-deep composite chain
- **THEN** the glyph either renders with its outline or fails with a diagnostic
- **AND** the embedded subset contains the components of every used glyph

### Requirement: Only the font's default kern, mark and mkmk lookups apply (RV4-42)

GPOS application SHALL select lookups through `ScriptList`, `FeatureList` and
`lookupFlag`, enabling the default-script `kern`, `mark` and `mkmk` features and
honouring `IgnoreMarks` and mark filtering sets. A lookup that belongs to another
script or feature MUST NOT be applied to ordinary text.

#### Scenario: A script-specific lookup is ignored

- **WHEN** a font declares a `PairPos` lookup only under a non-default script
- **THEN** the engine does not apply it to Latin or Cyrillic text
- **AND** `kern` lookups of the default script still apply

### Requirement: The font face cache distinguishes family lists (RV4-43)

The face cache MUST key on a family list unambiguously, so `"a b"` and `a, b` at
the same weight, slant and size cannot share an entry. A cache hit MUST return the
face the requested list resolves to.

#### Scenario: A family whose name contains a space

- **WHEN** a document uses `font-family: "a b"` and another uses `font-family: a, b`
  in the same conversion
- **THEN** each resolves to its own face
- **AND** the second request does not receive the first family's face

### Requirement: Every font load path is charged (RV4-44)

System-fallback parsing SHALL charge the work guard exactly like an `@font-face`
file, including the CFF conversion it may trigger. The budget MUST account for
every font the conversion parses, whatever the path that named it.

#### Scenario: A fallback font is parsed

- **WHEN** a document without `@font-face` resolves a family through the system
  fallback
- **THEN** the parse is charged to the guard
- **AND** an exhausted budget stops the fallback load with `resource/limit`

### Requirement: Generated text preserves shaped glyph offsets

Every painted shaped run, including headers, footers and markers, SHALL preserve
its horizontal and vertical glyph offsets until painting. Equivalent runs using
the same font and size MUST have equivalent relative glyph positions in body and
generated text. Unsupported mark attachment MUST remain a diagnostic rather than
an incorrectly positioned glyph.

#### Scenario: A stressed word in a margin box

- **WHEN** body text and a page margin box render `сло́во` with the same face and size
- **THEN** the mark has the same position relative to its base in both locations
- **AND** extracted text preserves each original combining sequence

#### Scenario: A mark has a vertical offset

- **WHEN** a generated text run contains a mark with nonzero vertical attachment
- **THEN** the mark is painted at the shaper's horizontal and vertical offsets

#### Scenario: Plain runs keep their output

- **WHEN** a run contains no positioned marks
- **THEN** its advances, alignment and text extraction remain unchanged

### Requirement: System fallback discovery preserves resource failures

System fallback discovery SHALL distinguish an unavailable candidate from a
per-file, aggregate or parser resource limit. A resource refusal MUST terminate
the conversion with `resource/limit`; discovery MUST NOT continue to another
family or silently select a different face because of that refusal. Cancellation
MUST be returned as the context error. A terminal resource error MUST NOT generate
a dependent `text/font` missing-family diagnostic.

#### Scenario: An aggregate cap refuses the fallback

- **WHEN** an injected available system font exceeds `MaxTotalFontBytes: 1`
- **THEN** both public entry points report `resource/limit` instead of `text/font`

#### Scenario: A variant exceeds the total

- **WHEN** the regular face fits but loading the selected family's variant crosses the aggregate limit
- **THEN** conversion fails with `resource/limit` rather than silently changing the selected typography

#### Scenario: An absent candidate permits discovery

- **WHEN** a candidate file does not exist and a later candidate is available within all limits
- **THEN** discovery may select the later candidate without a resource diagnostic

#### Scenario: Fallback discovery observes cancellation

- **WHEN** the context is cancelled during fallback loading
- **THEN** the call returns the context error without trying the remaining candidates

### Requirement: A GPOS FeatureList offset is bounds-checked (RV5-01)

Lookup selection SHALL read the `FeatureList` header and every feature record only
inside the GPOS table. An offset that does not fit the table MUST be treated as an
unusable feature list: the font SHALL load without kerning, and parsing MUST NOT
panic or interpret bytes outside the table.

#### Scenario: FeatureList past the table when the table is not last

- **WHEN** a font declares a valid `ScriptList` and a `FeatureList` offset beyond
  the table length, with file bytes after the table
- **THEN** the font loads without error
- **AND** no lookup is enabled from bytes outside the table
- **AND** kerning is not applied

#### Scenario: FeatureList past the table when the table ends the file

- **WHEN** the same malformed font has its GPOS table at the end of the file, so
  the table capacity ends with the file
- **THEN** the font loads or fails with a font error
- **AND** parsing does not panic

#### Scenario: Feature record past the table

- **WHEN** the `FeatureList` header fits but a feature record it points at does not
- **THEN** that feature contributes no lookups
- **AND** parsing does not read outside the table

#### Scenario: A well-formed FeatureList still selects lookups

- **WHEN** a font declares a valid `DFLT` or `latn` script whose `kern` feature
  points at a pair adjustment
- **THEN** that lookup is enabled and kerning is applied
- **AND** the existing kerning and GPOS tests stay green

### Requirement: A ScriptList with no ordinary script enables no lookups (RV5-28)

Lookup selection SHALL distinguish a missing or unusable `ScriptList` from a
usable list that names none of the ordinary scripts. The former MAY keep the
existing all-lookups fallback; the latter MUST enable no lookup, so a pair defined
only under another script cannot apply to Latin or Cyrillic text.

#### Scenario: An only-arab ScriptList

- **WHEN** a font declares one GPOS script `arab` with a `kern` pair of -300 and
  the document contains Latin `AV`
- **THEN** no lookup is enabled and the pair is not applied
- **AND** the font loads without a diagnostic

#### Scenario: An ordinary script still applies

- **WHEN** the same font also declares `DFLT` or `latn` with that pair
- **THEN** the pair is applied to Latin text

#### Scenario: No usable ScriptList

- **WHEN** the `ScriptList` offset is missing or out of bounds
- **THEN** the existing fallback keeps every lookup selectable
- **AND** a crafted single-script table without a list still applies its pairs

### Requirement: Exceeding the coverage cap returns resource/limit (RV5-29)

When lookup parsing would exceed the documented glyph-coverage cap, the parser
MUST return a `resource/limit` error. It MUST NOT return a partial set of enabled
lookups, so a font cannot silently lose kerning by declaring more coverage than
the cap.

#### Scenario: The fourth oversized lookup is refused

- **WHEN** a font declares four `kern` lookups and each covers 60,000 glyphs
- **THEN** the font load fails with `resource/limit`
- **AND** the engine does not apply a subset of three lookups

#### Scenario: A font within the cap loads

- **WHEN** the same lookups fit within the coverage cap
- **THEN** the font loads with all of them enabled
- **AND** kerning matches a font with equivalent tables

#### Scenario: Malformed is not the cap

- **WHEN** a lookup is truncated rather than over the cap
- **THEN** the font loads without that lookup, as the optional-table rule allows
- **AND** the outcome is not reported as `resource/limit`

### Requirement: A composite component beyond numGlyphs is rejected (RV5-30)

A composite glyph component whose glyph id is at or beyond `numGlyphs` MUST be an
error naming the glyph. The parser and the subsetter MUST NOT drop the component
and continue, and the embedded subset MUST NOT contain a dangling component
reference.

#### Scenario: A dangling component

- **WHEN** a used composite glyph references component id `numGlyphs`
- **THEN** the conversion reports an error naming the glyph
- **AND** no subset with the dangling reference is written

#### Scenario: A valid composite is unchanged

- **WHEN** every component id is inside `numGlyphs`
- **THEN** the outline and the embedded subset keep every component
- **AND** the output equals the current output for the same font

### Requirement: A truncated glyph is an error, not an empty outline (RV5-31)

The glyph reader SHALL distinguish an empty glyph (equal `loca` offsets, legal)
from a truncated glyph (malformed). A malformed simple or composite glyph MUST
produce an error naming the glyph; it MUST NOT return an empty or partial outline.

#### Scenario: A truncated simple glyph

- **WHEN** a used glyph's `loca` range ends inside its contour data
- **THEN** the conversion reports an error naming the glyph
- **AND** the glyph is not silently replaced by an empty outline

#### Scenario: A legal empty glyph

- **WHEN** a glyph's `loca` offsets are equal, as for `space`
- **THEN** it renders as an empty glyph with no error

#### Scenario: A truncated composite glyph

- **WHEN** a used composite's component data ends before its last component
- **THEN** the conversion reports an error
- **AND** the subset never embeds a partial component list as if complete

### Requirement: CFF conversion copies GDEF when present (RV5-32)

Converting a CFF font to TrueType SHALL copy the `GDEF` table when the source has
one, because glyph numbering is preserved. Mark classes and mark filtering sets
must survive the conversion so `IgnoreMarks` and `UseMarkFilteringSet` behave as
they do for the original font.

#### Scenario: A CFF font with GDEF

- **WHEN** an OpenType/CFF font with a 254-byte GDEF and `IgnoreMarks` pair lookups
  is loaded
- **THEN** the converted font contains `GDEF`
- **AND** the pair lookups skip mark glyphs as the source declares

#### Scenario: A CFF font without GDEF

- **WHEN** the source font has no `GDEF`
- **THEN** the converted font has none
- **AND** the table set otherwise matches the current conversion

### Requirement: A mark after a base attaches to that base (RV5-10)

When a non-mark glyph is stored, the attachment state MUST be reset so the next
combining mark is positioned from that base using `MarkBase` or `MarkLig`. A mark
MUST NOT reuse an anchor belonging to an earlier cluster, and a mark that has no
base in its run MUST remain the existing missing-mark diagnostic.

#### Scenario: The second acute in о́о́

- **WHEN** the text is `о` U+0301 `о` U+0301
- **THEN** the first acute is painted over the first `о`
- **AND** the second acute is painted over the second `о`, not over the first

#### Scenario: The second acute in а́б́

- **WHEN** the text is `а` U+0301 `б` U+0301
- **THEN** each acute uses the anchor of the base that immediately precedes it

#### Scenario: A mark at the start of a run

- **WHEN** a combining mark starts the shaped run and no base precedes it
- **THEN** the conversion reports the existing missing-mark diagnostic
- **AND** the mark is not painted at the pen position

### Requirement: Mark-to-mark attachment only follows an adjacent mark (RV5-10)

`MarkMark` SHALL be attempted only when the immediately preceding glyph is a mark
of the same cluster. When that anchor is absent, the shaper MUST fall back to the
current base's `MarkBase` anchor rather than to an older mark. A mark after a new
base MUST NOT use `MarkMark` at all.

#### Scenario: Two marks on one base

- **WHEN** the text is `о` U+0301 U+0302, two marks on one base, and the font
  provides a `MarkMark` anchor for the second
- **THEN** the second mark is attached mark-to-mark to the first
- **AND** both marks are painted above the same `о`

#### Scenario: A new base clears the mark state

- **WHEN** the sequence is base, mark, base, mark
- **THEN** the fourth glyph is attached with `MarkBase` or `MarkLig` to the second
  base
- **AND** no `MarkMark` lookup from the first cluster is used

#### Scenario: A font without a mark-to-mark anchor

- **WHEN** two consecutive marks follow one base and the font has no `MarkMark`
  anchor for the second
- **THEN** the shaper falls back to the current base's `MarkBase` anchor
- **AND** the document either renders with that anchor or reports the missing-mark
  diagnostic, never a stale position

### Requirement: Mark attachment stays over its base (RV5-10)

An attached mark's painted horizontal position SHALL lie over the glyph cell of
the base it attaches to, within the shaped run's extent. When the font's anchor
would place the mark over a different base or outside the run, the conversion
MUST report the existing missing-mark diagnostic instead of painting it there.

#### Scenario: An anchor outside the base

- **WHEN** a crafted font declares a mark anchor that places the mark before the
  run start or beyond the run end
- **THEN** the conversion reports the missing-mark diagnostic
- **AND** the glyph is not painted at the invalid position

#### Scenario: A normal anchor is unaffected

- **WHEN** the font's anchor places the mark inside the base's advance box
- **THEN** the mark is painted at that position with no diagnostic
- **AND** the run's advances are unchanged

### Requirement: GDEF class expansion is charged before it expands (RV5-15)

Parsing a GDEF table SHALL charge the shared work guard for every class range it
reads and for every glyph entry it expands, before the entry is written to the
mark map. A font whose GDEF expansion would exceed the budget MUST fail with
`resource/limit` instead of expanding the ranges.

#### Scenario: Overlapping class ranges stop at the budget

- **WHEN** a 12 KB font declares 2,000 overlapping class-3 ranges and the
  conversion runs with a budget smaller than their expansion
- **THEN** the font load stops with `resource/limit`
- **AND** the guard records work proportional to the ranges examined, not zero

#### Scenario: A normal font is unaffected

- **WHEN** a regular text font with a small GDEF is loaded under the default
  limits
- **THEN** it loads with its mark data and no diagnostic
- **AND** the charge is far below the default budget

#### Scenario: Entries, not only ranges, are charged

- **WHEN** one range covers 60,000 glyphs and another covers one
- **THEN** the charge for the wide range is larger than the charge for the narrow
  one
- **AND** a budget that admits the narrow range refuses the wide one

### Requirement: GDEF parsing observes cancellation (RV5-15)

The parser SHALL check the guard between class ranges and mark-set coverage
entries, so a cancelled context stops the parse instead of returning a font after
the whole table. The returned error MUST be the context error, not a
`resource/limit` diagnostic.

#### Scenario: Cancellation in the middle of a parse

- **WHEN** the context is cancelled 20 ms after a GDEF-heavy font starts parsing
- **THEN** the load returns the context error promptly
- **AND** it does not return a successfully parsed font

#### Scenario: A pre-cancelled context

- **WHEN** the context is already cancelled before the font is opened
- **THEN** the load returns the context error without parsing the GDEF table

### Requirement: GDEF expansion is capped (RV5-15)

A font's total expanded mark entries SHALL be bounded by a documented cap. A font
whose ranges expand beyond the cap MUST fail with `resource/limit`; the parser
MUST NOT keep a partial mark set that silently changes which glyphs are treated
as marks.

#### Scenario: A font at the format maximum

- **WHEN** a GDEF table declares 65,535 overlapping class-3 ranges
- **THEN** the load fails with `resource/limit` after bounded work
- **AND** no mark map larger than the cap is allocated

#### Scenario: A cap crossing is not a partial parse

- **WHEN** the expansion crosses the cap in the middle of a table
- **THEN** the whole font is refused
- **AND** the converter never applies a mark set built from only the first ranges

### Requirement: A GDEF coverage limit is propagated (RV5-15)

A `resource/limit` returned while reading a GDEF mark-set coverage SHALL be
propagated, not swallowed. The conversion MUST stop with `resource/limit`. A
malformed optional table that is not a resource refusal MUST keep the existing
degrade behaviour: the font loads without GDEF mark data.

#### Scenario: A mark-set coverage over the cap

- **WHEN** a GDEF mark set covers more glyphs than the font coverage cap allows
- **THEN** the font load fails with `resource/limit`
- **AND** the parser does not continue with that set dropped

#### Scenario: Malformed GDEF still degrades

- **WHEN** the GDEF table is truncated or inconsistent but does not cross a
  resource bound
- **THEN** the font loads without mark data
- **AND** no diagnostic is required for the optional table
