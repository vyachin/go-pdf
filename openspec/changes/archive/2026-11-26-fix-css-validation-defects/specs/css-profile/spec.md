# Spec Delta

## ADDED Requirements

### Requirement: A non-positive line-height is rejected (RV5-21)

`line-height` SHALL reject a negative or zero value in every unit the profile
accepts, including percentages and `em` lengths. Such a value MUST produce
`css/unsupported-value` for the declaration and MUST NOT be replaced at layout time
by an ascent-derived line height.

#### Scenario: A negative percentage line height

- **WHEN** a document contains `<p style="line-height:-50%">A<br>B</p>`
- **THEN** validation reports `css/unsupported-value` naming the declaration
- **AND** `Convert` returns an error and no PDF

#### Scenario: A zero percentage line height

- **WHEN** a rule declares `line-height: 0%`
- **THEN** validation rejects it with `css/unsupported-value`

#### Scenario: A negative em line height

- **WHEN** a rule declares `line-height: -2em`
- **THEN** validation rejects it with `css/unsupported-value`

#### Scenario: Positive forms stay accepted

- **WHEN** declarations use `line-height: 1.5`, `18pt` or `120%`
- **THEN** validation accepts each and the rendered line height follows the value
- **AND** the existing line-height tests stay green

### Requirement: Border colours default to currentColor (RV5-22)

The initial border colour SHALL be `currentColor`, resolved from the element's
`color` when the border is painted. The `currentColor` keyword MUST be accepted
wherever a border colour is accepted, and an explicitly declared border colour in
the `border` shorthand MUST win over the initial value.

#### Scenario: A border without an explicit colour

- **WHEN** a rule declares `color:#c00; border:1px solid`
- **THEN** the painted border is red
- **AND** it is not black

#### Scenario: An explicit border colour wins

- **WHEN** the same rule also declares `border:1px solid #00f`
- **THEN** the border is blue
- **AND** `currentColor` is not applied over it

#### Scenario: The keyword itself

- **WHEN** a rule declares `border:1px solid currentColor` and the text is red
- **THEN** validation accepts the declaration and the border is red

### Requirement: Inline declaration diagnostics carry the element position (RV5-23)

A declaration inside a `style` attribute that validation rejects SHALL carry the
file, line and character column of the element that owns the attribute. A zero
position MUST NOT be reported when that position is known.

#### Scenario: A rejected inline declaration

- **WHEN** a document contains `<div style="font-size:1pt"><p style="font-size:0.01%">x</p></div>`
- **THEN** the diagnostic names the line and column of the `<p>` element
- **AND** it does not report `:0:0`

#### Scenario: The same position as an attribute diagnostic

- **WHEN** the rejected declaration is on `<p style="display:flex">` in a multi-line document
- **THEN** its position matches the element's position on the correct line

### Requirement: A page margin box is only valid inside @page (RV5-24)

A page margin box at-rule (`@top-center`, `@bottom-left` and the other margin-box
names) SHALL be accepted only inside an `@page` rule. At any other position it MUST
produce `css/unsupported-at-rule`, and its declarations MUST NOT be painted or
change page geometry.

#### Scenario: A margin box at the top level

- **WHEN** a stylesheet declares `@top-center { content: "HEADER" }` outside `@page`
- **THEN** validation reports `css/unsupported-at-rule`
- **AND** the extracted PDF text does not contain "HEADER"

#### Scenario: A margin box inside a media rule but outside @page

- **WHEN** the rule is `@media print { @top-center { content: "HEADER" } }`
- **THEN** validation reports `css/unsupported-at-rule`

#### Scenario: A margin box inside @page still works

- **WHEN** the rule is `@page { @top-center { content: "HEADER" } }`
- **THEN** validation accepts it and the header is painted on each page

### Requirement: Each parser problem is counted once (RV5-25)

Each parser problem SHALL be counted once against a stylesheet's problem cap. A
stylesheet with fewer distinct problems than the cap MUST report all of them, and
`resource/limit` MUST appear only when the number of distinct problems exceeds the
cap.

#### Scenario: Sixty bad declarations

- **WHEN** one rule contains sixty malformed declarations and the cap is one hundred
- **THEN** validation reports sixty `css/syntax` problems
- **AND** it does not report `resource/limit`

#### Scenario: More problems than the cap

- **WHEN** one stylesheet contains more than one hundred distinct problems
- **THEN** validation stores one hundred problems plus one `resource/limit`
- **AND** the limit message names the cap

#### Scenario: Problems inside a nested block

- **WHEN** sixty malformed declarations sit inside one `@media` block
- **THEN** each is reported once
- **AND** the count is not doubled when the block is merged into its parent

#### Scenario: The stray-brace guard stays green

- **WHEN** a megabyte of `}` characters is parsed
- **THEN** the call still returns `resource/limit` with a bounded diagnostic list
- **AND** the existing RV4-02 test stays green
