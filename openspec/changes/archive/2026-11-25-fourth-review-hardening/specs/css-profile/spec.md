# Spec Delta

## ADDED Requirements

### Requirement: Stylesheet problems are capped and charged (RV4-02)

The CSS tokenizer and parser SHALL charge the work guard for the input they read
and MUST cap the number of stored problems, stopping with a single `resource/limit`
once the cap is reached. A stylesheet MUST NOT be able to produce one diagnostic per
input byte, and the returned diagnostic list MUST stay bounded independently of the
input size.

#### Scenario: A stylesheet of stray braces

- **WHEN** a document contains a `<style>` element of one megabyte consisting of
  `}` characters
- **THEN** the call returns `resource/limit` after a bounded number of diagnostics
- **AND** the memory and the error text stay proportional to the cap, not to the
  input

### Requirement: Adjacent-sibling matching is bounded and charged (RV4-07)

Matching a `+` selector MUST NOT scan every preceding sibling for each element. The
hop SHALL be constant time or charged per sibling visited, so the cost stays within
the documented bound of compounds times ancestor depth.

#### Scenario: Many siblings with an adjacent selector

- **WHEN** a stylesheet contains `p + p` and the document has 80000 sibling
  paragraphs
- **THEN** matching time stays close to the `p p` case
- **AND** a small `MaxOperations` stops the conversion before minutes of matching

### Requirement: A CSS-wide keyword is not a font family (RV4-11)

`font-family` SHALL reject `inherit`, `initial`, `unset` and `revert`, or implement
`inherit`. An identifier that names a CSS-wide keyword MUST NOT be stored as a
family name and MUST NOT fall through to a system fallback font.

#### Scenario: inherit in a font family

- **WHEN** a document declares `p{font-family:inherit}` with a declared family on
  the parent
- **THEN** the paragraph either inherits that family or is rejected with
  `css/unsupported-value`
- **AND** it is never rendered in an undeclared system font

### Requirement: media is a supported attribute of style (RV4-18)

The `media` attribute SHALL be accepted on `<style>` exactly as on `<link>`, and a
value that applies to print MUST apply the sheet. Profile validation MUST NOT report
it as an unsupported attribute, and `Strict` MUST accept a document that uses it.

#### Scenario: A print-only style element

- **WHEN** a document contains `<style media="print">p{color:#00ff00}</style>`
- **THEN** validation reports no `html/unsupported-attribute`
- **AND** the rule applies, including with `Strict: true`

### Requirement: A pseudo-element is only valid in the final compound (RV4-19)

A selector that carries a pseudo-element in any compound other than the last SHALL
be rejected with `css/unsupported-selector`. A pseudo-element MUST NOT match or
style the element selected by a following compound.

#### Scenario: A pseudo-element before a descendant

- **WHEN** a stylesheet contains `a::after span { color:#ff0000 }`
- **THEN** validation rejects the selector
- **AND** the span is not painted red

### Requirement: A percentage height is rejected or resolved (RV4-20)

`height` SHALL either reject a percentage with `css/unsupported-value` or resolve it
against the containing block. A declaration the profile validates MUST NOT be
silently ignored by layout.

#### Scenario: A percentage height

- **WHEN** a document declares `height: 50%` on a block inside a parent with a
  declared height
- **THEN** either the height resolves to half the containing block or validation
  rejects the value
- **AND** the rendered box is never silently treated as `height: auto`

### Requirement: A relative path resolves inside the confined reader's root (RV4-36)

`Root.Read`, `Root.Resolve` and `Root.Size` SHALL interpret a relative path relative
to the configured root directory, not the process working directory, and `Size` MUST
stat through the root handle. A path inside the root MUST NOT be reported as an
escape because the process working directory differs.

#### Scenario: A relative path inside the root

- **WHEN** a root is opened for a directory and a caller reads `sub/file.txt`
  relative to it
- **THEN** the file resolves inside the root
- **AND** the same call from another working directory behaves identically

### Requirement: A nested at-rule the parent cannot represent is an error (RV4-46)

An at-rule nested where its parent cannot represent it SHALL produce a problem
(`css/unsupported-at-rule`) instead of being dropped, and `@page` inside `@page`
MUST be rejected.

#### Scenario: A media rule inside a font face

- **WHEN** a stylesheet contains `@font-face { ... @media print { p{color:red} } }`
- **THEN** validation reports an unsupported nested at-rule
- **AND** the rule is not silently discarded

### Requirement: A compound with two ids cannot match (RV4-47)

A compound that contains more than one id SHALL be rejected, or every id MUST be
required to match and counted in the specificity. A rule such as `#x#y` MUST NOT
match an element that carries only one of the ids.

#### Scenario: Two ids in one compound

- **WHEN** a stylesheet contains `#x#y { color:#ff0000 }` and the document has
  `<p id="y">`
- **THEN** the paragraph is not matched by that rule
- **AND** the rule's specificity is not reduced to a single id

### Requirement: Identifiers decode backslash escapes (RV4-48)

The tokenizer SHALL decode CSS escapes in identifiers and hashes as it already does
in strings, so `caf\e9` names `café` in a family name and in a class selector.

#### Scenario: An escaped family name

- **WHEN** a stylesheet contains `font-family: caf\e9`
- **THEN** the family resolves to `café`
- **AND** the same escape in a selector matches the class `café`

### Requirement: A media query list is parsed as a list (RV4-49)

Validation and stylesheet collection SHALL share one media-list parser.
`@media print, screen` MUST apply for print instead of being rejected, and a list
that contains `print` MUST be treated as applying.

#### Scenario: A media list with print

- **WHEN** a stylesheet contains `@media print, screen { p{color:red} }`
- **THEN** validation accepts the rule
- **AND** the declarations apply to the print layout

### Requirement: A declaration is never dropped silently (RV4-50)

Cascade application MUST NOT discard a declaration that validation accepted without
a diagnostic. Validation SHALL use the resolved parent font size, so a value that
can round to zero at render time is rejected during validation instead.

#### Scenario: A font size that rounds to zero

- **WHEN** a document sets a tiny percentage font size on a child of a one-point
  parent
- **THEN** validation reports `css/unsupported-value` for the declaration
- **AND** the paragraph is not silently rendered at the parent size

### Requirement: always is documented as a break value (RV4-61)

The profile SHALL document `always` as an accepted alias of `page` for
`break-before` and `break-after`, or reject it. The property table in
`docs/profile.md` and the parser MUST agree.

#### Scenario: break-before always

- **WHEN** a template uses `break-before: always`
- **THEN** the parser treats it as `break-before: page`
- **AND** the documented values for the property include `always`

### Requirement: Grammar pieces are shared and their policy is explicit (IMP-12)

Media lists, CSS-wide keywords, identifier escapes and nested at-rules SHALL each
have one implementation and one documented policy, used by validation and by
stylesheet collection. A second parser for any of them MUST NOT exist.

#### Scenario: A new grammar rule (IMP-12)

- **WHEN** a media list, an escape or a nested at-rule is supported
- **THEN** validation and collection use the same parser and agree
- **AND** the policy is stated in `docs/profile.md`

