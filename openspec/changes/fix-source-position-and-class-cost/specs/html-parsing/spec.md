# Spec Delta

## ADDED Requirements

### Requirement: Source position and element path lookup are linear (RV5-54)

Computing a node's source position SHALL cost at most the bytes since the previous
position query on the same line, not a rescan from the line start. Building a
node's element path SHALL cost time and allocation linear in the path length. Both
SHALL be charged to the shared work budget, so a one-line document cannot make
diagnostics or layout provenance quadratic without reaching `resource/limit`.

#### Scenario: Many positions on one long line

- **WHEN** a 112 KB document is one line and 8,000 positions are requested
- **THEN** the total cost grows linearly with the number of queries and their
  offsets
- **AND** the work charged is proportional to the bytes examined

#### Scenario: A minified document reaches the diagnostic stage

- **WHEN** a one-line document produces 8,000 validation diagnostics with a work
  budget sized for a multiline document of the same size
- **THEN** validation completes within that budget
- **AND** the diagnostic positions are the same as the multiline equivalent

#### Scenario: An element with very many classes

- **WHEN** one element carries 40,000 class tokens and a diagnostic builds its path
- **THEN** the path is built in time and allocation linear in its length
- **AND** the charged work is proportional to the bytes written

### Requirement: Class tokens split on ASCII whitespace only (RV5-55)

The class attribute SHALL be split on ASCII whitespace (space, tab, line feed, form
feed and carriage return) and MUST NOT treat any other Unicode space as a
separator. A no-break space SHALL stay part of its token. The token slice SHALL be
computed once per node and reused by every class selector match.

#### Scenario: A class token containing a no-break space

- **WHEN** an element carries `class="a&#160;b"`
- **THEN** its class tokens are the single token `a\u00a0b`
- **AND** the selector `.a` does not match it
- **AND** a selector for the whole token does

#### Scenario: ASCII whitespace still separates tokens

- **WHEN** an element carries `class="a \t b\nc"` (with ASCII whitespace)
- **THEN** its tokens are `a`, `b` and `c`

#### Scenario: Repeated matching does not re-split

- **WHEN** fifty class selectors are matched against one element with 20,000
  classes
- **THEN** the class attribute is split once
- **AND** every match reads the cached token slice
