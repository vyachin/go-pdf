# Spec Delta

## REMOVED Requirements

### Requirement: The import runs only after the strict profile failed

**Reason:** Browser compatibility lowering is removed; validation and conversion
MUST use the closed profile without a rewriting fallback.

**Migration:** Callers MUST remove `ImportBrowser` from option literals and author
templates within the documented profile.

#### Scenario: An unsupported document without a fallback

- **WHEN** an HTML document outside the profile is passed to conversion or validation
- **THEN** it SHALL fail with profile diagnostics rather than being rewritten and retried

### Requirement: Layout constructs are lowered to the profile

**Reason:** Flex wrappers, absolute-image replacement and border-box dimension
rewriting are part of the removed compatibility layer.

**Migration:** Templates MUST express their layout using supported blocks, tables,
background images and content-box dimensions.

#### Scenario: A formerly importable flex column

- **WHEN** a template declares `display: flex; flex-direction: column`
- **THEN** it SHALL fail profile validation without generating block wrappers

#### Scenario: A positioned cover image

- **WHEN** a template positions an image absolutely over its ancestor
- **THEN** it SHALL fail profile validation without turning the image into a background

### Requirement: Lowering uses resolved border and margin values

**Reason:** Import-specific border-box calculations and flex-margin conversions
are removed with the layouts they served.

**Migration:** Templates MUST use supported box dimensions and margins directly;
native support for `em` borders and profile-supported margins SHALL remain available.

#### Scenario: Border-box is not translated

- **WHEN** a template declares `box-sizing: border-box` with an explicit dimension and an `em` border
- **THEN** it SHALL fail profile validation rather than rewriting its dimensions

### Requirement: Values and screen-only rules are lowered

**Reason:** Browser units, variables, math functions, text transforms, cosmetic
property removal and media-rule removal are no longer preprocessing features.

**Migration:** Templates MUST supply supported units and values, express text in
its intended case, and follow the documented property and at-rule allow-list.

#### Scenario: A browser-only value or declaration

- **WHEN** a template uses `rem`, `var()`, `text-transform` or `box-shadow`
- **THEN** it SHALL receive profile errors without substituting values, altering text or dropping declarations

#### Scenario: An unsupported screen media rule

- **WHEN** a stylesheet contains `@media (max-width: 793px)`
- **THEN** it SHALL receive a profile error rather than silently dropping the rule

### Requirement: Constructs outside the subset are rejected

**Reason:** The importer-specific subset and its diagnostic codes are retired;
unsupported syntax is governed by the existing CSS profile.

**Migration:** Consumers MUST handle the normal profile diagnostics for
unsupported constructs instead of expecting `browser/unsupported-*` codes.

#### Scenario: A flex row has a profile error

- **WHEN** a template declares a flex row
- **THEN** it SHALL fail with profile diagnostics and no `browser/unsupported-flex` diagnostic

### Requirement: Every transformation is reported

**Reason:** No browser lowering remains to report.

**Migration:** Consumers MUST use profile diagnostics and remove expectations of
`browser/*` warnings or errors. Conversion and validation SHALL emit no such codes.

#### Scenario: Diagnostics after removal

- **WHEN** a document previously requiring browser import is converted or validated
- **THEN** its report SHALL contain profile errors and no `browser/*` diagnostics

### Requirement: The certificate is the acceptance fixture

**Reason:** Converting the browser-oriented certificate without edits is no
longer a supported capability.

**Migration:** The certificate SHALL serve as a rejection fixture. Supported
certificate rendering SHALL use the separate profile-compatible visual fixture.

#### Scenario: The browser certificate is rejected

- **WHEN** the committed browser certificate is converted or validated
- **THEN** it SHALL fail with deterministic profile diagnostics and no PDF

#### Scenario: The reference command rejects the certificate

- **WHEN** the reference command is given the browser certificate
- **THEN** it SHALL exit with code `1`, report profile diagnostics and create no output PDF
