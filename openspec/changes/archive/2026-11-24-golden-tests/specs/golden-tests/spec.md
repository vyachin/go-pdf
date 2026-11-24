# Spec Delta

## Purpose

Keeps the converter's reading of a document and the appearance of the rendered
page frozen in reviewable artifacts, so a change in parsing, cascade or layout
becomes visible in review instead of in a customer's template.

## ADDED Requirements

### Requirement: Document snapshot for every fixture

The suite SHALL keep one committed document snapshot per fixture. A snapshot is
a canonical XML serialization of the document model the converter prepares:
elements with their effective styles, `@page` setup, margin boxes, `@font-face`
declarations, metadata and diagnostics. Producing a snapshot MUST NOT require a
system font, a PDF renderer or a PDF writer.

#### Scenario: Deterministic bytes

- **WHEN** the same fixture is snapshotted twice with the same options
- **THEN** both snapshots are byte identical

#### Scenario: Effective styles are recorded once

- **WHEN** a property is inherited and its computed value on a child equals the
  computed value on its parent
- **THEN** the snapshot omits it from the child, and the format documents that
  an omitted inherited property means "the parent's value"

#### Scenario: Page setup is part of the snapshot

- **WHEN** the document sets `@page` size, margins or margin boxes
- **THEN** the snapshot records the resolved values, including `counter(page)`
  and `counter(pages)` content as unresolved tokens

#### Scenario: Diagnostics are part of the snapshot

- **WHEN** validation produces warnings or errors for a fixture
- **THEN** they appear in the snapshot in a stable sorted order with code,
  severity, position and element path

#### Scenario: A snapshot needs no system font

- **WHEN** no system font is available and the fixture declares no `@font-face`
- **THEN** the snapshot is still produced, because the document model does not
  depend on font metrics

#### Scenario: Snapshot bytes are stable across text formatting

- **WHEN** the serializer emits attributes and elements
- **THEN** the order of attributes and elements is fixed by the format, so a
  re-serialization without a model change produces identical bytes

### Requirement: Page render snapshots for the visual set

The suite SHALL keep a committed raster for every page of every fixture in the
visual set. A page MUST be rasterized from the produced PDF at a fixed
resolution and compared pixel by pixel with its committed image.

#### Scenario: Unchanged document renders identically

- **WHEN** a visual fixture is converted and rasterized
- **THEN** every page has the committed size and zero differing pixels

#### Scenario: A small visual change is detected

- **WHEN** a change moves or adds a single glyph on a page
- **THEN** the test fails and reports the number of differing pixels and the
  bounding box that contains them

#### Scenario: The renderer is recorded

- **WHEN** snapshots are regenerated
- **THEN** the identity and version of the rasterizer are recorded next to them

#### Scenario: A renderer change is reported, not mistaken for a layout bug

- **WHEN** the suite runs in the strict CI mode and the current rasterizer does
  not match the recorded one
- **THEN** the test fails with a message that names both versions and the
  regeneration command

#### Scenario: A missing rasterizer skips the layer locally

- **WHEN** the rasterizer is not installed and strict CI mode is off
- **THEN** the render layer is skipped and the rest of the suite still runs

### Requirement: Snapshot maintenance

Snapshots MUST be regenerable from the fixtures and MUST NOT decay silently.

#### Scenario: Regeneration is explicit

- **WHEN** the suite runs with the update flag
- **THEN** it rewrites the snapshots of the fixtures it processed and leaves
  every other snapshot unchanged

#### Scenario: A fixture without a snapshot fails

- **WHEN** a fixture has no snapshot and the update flag is off
- **THEN** the test fails and names the command that creates the snapshot

#### Scenario: A snapshot without a fixture fails

- **WHEN** a committed snapshot has no matching fixture
- **THEN** the test fails and names the stale file

### Requirement: Fixture corpus covers the profile

The suite SHALL fail when a supported property or element has no fixture that
exercises it, so a golden cannot be green for a feature it never runs.

#### Scenario: Uncovered property

- **WHEN** a property is listed in the profile but no fixture declares it
- **THEN** the coverage test fails and names the property

#### Scenario: Uncovered element

- **WHEN** an element is supported by the profile but appears in no fixture
- **THEN** the coverage test fails and names the element

### Requirement: Snapshot failure reporting

A mismatch MUST produce enough information to review it without rerunning the
suite.

#### Scenario: Document mismatch

- **WHEN** a document snapshot differs
- **THEN** the failure shows the first differing lines with line numbers and the
  path of the produced snapshot

#### Scenario: Render mismatch

- **WHEN** a page render differs
- **THEN** the suite writes the produced image, the committed image and a
  difference image, and reports their paths
