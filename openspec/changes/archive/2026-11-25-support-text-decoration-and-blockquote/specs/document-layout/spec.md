# Spec Delta

## ADDED Requirements

### Requirement: Text decorations and quotations are painted

A decorated text run SHALL be painted with its decoration: `underline` draws a line
below the baseline, `line-through` crosses the text at the middle of its
x-height, and both use the run's colour. A decoration SHALL cover the painted run
rather than each glyph separately. A `blockquote` SHALL be laid out as a block with
a left and right indent, and its content SHALL keep block layout.

#### Scenario: An underlined word
- **WHEN** a paragraph contains `<u>underlined</u>` at a known position
- **THEN** the page has one filled rectangle under that run, in the text colour and as wide as the run

#### Scenario: A struck-through word
- **WHEN** a paragraph contains `<s>struck</s>`
- **THEN** the rectangle crosses the run at the middle of the x-height rather than below the baseline

#### Scenario: A quote is indented
- **WHEN** a document contains `<blockquote>quoted text</blockquote>`
- **THEN** the quote's text starts inside the left indent and ends before the right indent, and its block layout is preserved
