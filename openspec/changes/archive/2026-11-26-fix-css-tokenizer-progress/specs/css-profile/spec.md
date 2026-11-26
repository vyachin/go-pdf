# Spec Delta

## ADDED Requirements

### Requirement: CSS tokenization always progresses and is charged (RV5-02)

Every tokenizer step SHALL consume at least one input byte or stop the parse with a
`css/syntax` problem, so no input can loop. Declaration parsing MUST count toward the
documented work budget and observe cancellation, so a stylesheet MUST NOT spend the
budget one empty token at a time; an exhausted budget or a cancelled context MUST
stop the call promptly with `resource/limit` or the context error.

#### Scenario: A style attribute ends with a lone backslash

- **WHEN** a document contains `<p style="color:red\">x</p>` and `Validate` or
  `Convert` is called
- **THEN** the call returns and reports `css/syntax` for the attribute
- **AND** it does not hang and produces no partial PDF

#### Scenario: A style element ends with a lone backslash

- **WHEN** a document contains `<style>p{color:red\</style>`
- **THEN** the call returns with `css/syntax` or `resource/limit`
- **AND** the tokens it stored stay bounded by the work budget instead of growing
  without limit

#### Scenario: A cancelled context stops declaration parsing

- **WHEN** the caller passes an already cancelled context for a document with a
  style attribute
- **THEN** the call returns `context.Canceled` instead of parsing to completion

#### Scenario: A tiny work budget bounds a stray backslash

- **WHEN** the caller sets a small `MaxOperations` and the document contains a style
  attribute or stylesheet ending in many lone backslashes
- **THEN** the call returns `resource/limit` after a bounded amount of work

#### Scenario: Well-formed CSS is unaffected

- **WHEN** documents and stylesheets use valid escapes such as `content: "\2014"` or
  `font-family: caf\e9`
- **THEN** they parse and render exactly as before
- **AND** the existing CSS tests stay green
