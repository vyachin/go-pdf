# browser-import Specification

## Purpose

Defines the opt-in compatibility layer that lowers browser-oriented HTML and CSS to the closed profile without relaxing the profile contract. The import exists so that real templates, such as a certificate exported from a design tool, can be converted by the same deterministic engine, while a strict profile template is never rewritten.

## Requirements

### Requirement: The import runs only after the strict profile failed

The engine SHALL validate the document as written first. The browser import MUST run only when that validation produced errors, the caller enabled `Options.ImportBrowser`, and the document is not in strict mode. A document that already passes the profile SHALL be converted without any lowering. The reference command SHALL enable the import for the single conversion it performs, so that a browser-oriented template converts without an extra flag.

#### Scenario: A valid profile template
- **WHEN** a document inside the profile is converted with the import enabled
- **THEN** no lowering runs and the output is identical to a conversion with the import disabled

#### Scenario: A browser document with the import enabled
- **WHEN** a browser document is converted with `Options.ImportBrowser`
- **THEN** the strict errors are replaced by the import report and the rewritten document is validated again

#### Scenario: Strict mode
- **WHEN** the same browser document is converted with `Options.Strict` or validated with `Validate`
- **THEN** the original profile diagnostics are reported and no PDF is returned

### Requirement: Layout constructs are lowered to the profile

The import SHALL lower a single-line flex column to block layout, a box-covering absolutely positioned image to a background image, and `box-sizing: border-box` with an explicit dimension to the content-box model. A flex item's vertical margins SHALL become padding on a generated wrapper, so the flex spacing survives without margin collapsing. The positioned ancestor of the lowered background image MUST have an explicit height.

#### Scenario: A flex column
- **WHEN** a container is a flex column with centred items and margins
- **THEN** it becomes a block whose item wrappers keep the same vertical spacing

#### Scenario: A full-page background image
- **WHEN** an absolutely positioned image covers a positioned ancestor with an explicit height
- **THEN** the image becomes the ancestor's cover background and the `<img>` element is removed

#### Scenario: Border-box sizing
- **WHEN** an element declares `box-sizing: border-box`, an explicit height and bottom padding
- **THEN** the padding is subtracted from the content height and the border box keeps its declared size

### Requirement: Values and screen-only rules are lowered

The import SHALL resolve `rem` against the root font size, `vw`, `vh`, `vmin` and `vmax` against the page area, a whole-value `min()`, `max()` or `clamp()` to a single length, and `var()` against custom properties declared on the document element. It SHALL drop `@media` rules that cannot apply to print, rewrite `background: <colour>` to `background-color`, apply `text-transform` to the text, and drop the cosmetic properties that have no profile meaning.

#### Scenario: Relative units and variables
- **WHEN** the document uses `rem`, `vh`, `min()` or `var(--accent)`
- **THEN** each declaration is rewritten to a supported absolute value before validation

#### Scenario: A screen media query
- **WHEN** the document contains `@media (max-width: 793px)`
- **THEN** the rule is dropped from the print document and a `browser/media` warning is reported

#### Scenario: Text transform
- **WHEN** the document declares `text-transform: uppercase` on a title
- **THEN** the title text is uppercased and the property is dropped

#### Scenario: A cosmetic declaration
- **WHEN** the document declares `box-shadow` or `isolation`
- **THEN** the declaration is dropped with a `browser/dropped-property` warning

### Requirement: Constructs outside the subset are rejected

The import SHALL reject a construct it cannot lower instead of approximating it. A flex row, a non-`flex-start` justification, wrapping, a gap, `flex-grow`, `align-self`, `order`, an unsupported `align-items` value, an absolutely positioned element that is not a box-covering image, a missing positioned ancestor and an unsupported `text-transform` value MUST produce an error diagnostic and stop the conversion.

#### Scenario: A flex row
- **WHEN** a container is a flex row
- **THEN** conversion fails with `browser/unsupported-flex`

#### Scenario: Wrapping or a gap
- **WHEN** a flex column declares `flex-wrap: wrap` or `gap`
- **THEN** conversion fails with `browser/unsupported-flex` instead of dropping the layout difference

#### Scenario: An absolutely positioned text block
- **WHEN** an element other than a box-covering `<img>` is positioned absolutely
- **THEN** conversion fails with `browser/unsupported-position`

#### Scenario: A missing positioned ancestor
- **WHEN** a cover image has no positioned ancestor with an explicit height
- **THEN** conversion fails with `browser/unsupported-position`

### Requirement: Every transformation is reported

Every lowering SHALL be reported as a warning with the declaration or element position: `browser/flex`, `browser/background`, `browser/box-sizing`, `browser/text-transform`, `browser/media`, `browser/dropped-property`. The report MUST NOT be silently suppressed in normal mode.

#### Scenario: A successful import
- **WHEN** a browser document is imported and converted
- **THEN** the result carries a warning for every transformation and no errors

### Requirement: The certificate is the acceptance fixture

`testdata/browser/certificate.html` SHALL be converted without edits into one A4 page with the expected text and images, the import SHALL report the flex and background lowerings, the plain profile SHALL still reject the file, and two imports MUST produce byte-identical PDFs.

#### Scenario: The certificate
- **WHEN** the committed browser certificate is converted with the import enabled
- **THEN** the PDF has one A4 page, contains the certificate number and title text, embeds the background, logo and QR images, and is byte-identical on a second run

#### Scenario: The profile still rejects the browser file
- **WHEN** the certificate is converted without the import
- **THEN** conversion fails with profile diagnostics and no PDF is written
