# Spec Delta

## ADDED Requirements

### Requirement: An empty inline anchor keeps its page target (RV5-41)

An inline element with an `id` SHALL register its anchor at the flow position it
occupies, even when it generates no visible content. An empty `<span id>` or
`<a id>` MUST NOT disappear from the anchor map, and a reference to it MUST resolve
to the page it occupies instead of reporting `pagination/unknown-target`.

#### Scenario: An empty anchor on the second page

- **WHEN** a two-page document has an empty `<span id="target">` on page 2 and an
  `a` whose generated content uses `target-counter(attr(href), page)` for it
- **THEN** the generated content shows page 2
- **AND** no `pagination/unknown-target` is reported

#### Scenario: An empty anchor inside a paragraph

- **WHEN** a document contains `<p>text <a id="x"></a></p>` and
  `<p><span id="split"></span>tail</p>`
- **THEN** both ids are present in the anchor map
- **AND** the paragraph's visible line count and the page count are unchanged

#### Scenario: An anchor before a word that wraps

- **WHEN** an empty anchor sits before a word that moves to the next line
- **THEN** the anchor resolves to the page of that word

### Requirement: A page-ending line is not justified (RV5-42)

A justified line that ends a page SHALL NOT be stretched to the content edge. Only
the final line of a paragraph MUST remain unstretched; each page's closing line
MUST keep its natural width and MUST NOT receive the extra space of justification.

#### Scenario: A justified paragraph over three pages

- **WHEN** an eighty-word paragraph with `text-align: justify` is laid out on
  200×90 pt pages
- **THEN** the last line painted on each page is narrower than the content box
- **AND** the paragraph's final line is also unstretched
- **AND** the lines between them are still justified

#### Scenario: A single-page justified paragraph

- **WHEN** a justified paragraph fits on one page
- **THEN** only its final line is unstretched

#### Scenario: A line before a forced break

- **WHEN** a forced break follows a justified line
- **THEN** that line is not stretched
