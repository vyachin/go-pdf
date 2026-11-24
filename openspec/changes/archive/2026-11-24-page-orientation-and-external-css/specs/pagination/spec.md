# Spec Delta

## MODIFIED Requirements

### Requirement: Page size and margins

The engine SHALL determine the page size from `@page { size }`, and in its absence — from the launch parameter. The names `A4`, `A5`, `Letter`, explicit sizes in length units and the orientation `portrait` or `landscape` are supported — separately or together with a name. Orientation without a size name applies to the size from the launch parameters. Margins from `@page` SHALL reduce the content area and SHALL NOT overlap the headers and footers.

#### Scenario: Size from CSS
- **WHEN** the template declares `@page { size: A4 }` and the run specifies A5
- **THEN** the PDF is created in A4 format

#### Scenario: Default size
- **WHEN** the template does not declare `@page size` and the run specifies A5
- **THEN** the PDF is created in A5 format

#### Scenario: Landscape orientation of a named size
- **WHEN** the template declares `@page { size: A4 landscape }`
- **THEN** the page has width 297 mm and height 210 mm
- **AND** the headers, footers and margins are placed on the new sides

#### Scenario: Portrait orientation
- **WHEN** the template declares `@page { size: A4 portrait }`
- **THEN** the page has width 210 mm and height 297 mm

#### Scenario: Orientation without a named size
- **WHEN** the template declares `@page { size: landscape }` and the run specifies A5
- **THEN** the page has width 210 mm and height 148 mm

#### Scenario: Orientation together with explicit sizes
- **WHEN** the template declares `@page { size: 210mm 297mm landscape }`
- **THEN** validation fails because the CSS grammar does not allow orientation together with lengths

#### Scenario: Unknown word in the size
- **WHEN** the template declares `@page { size: A4 sideways }`
- **THEN** validation fails with an indication of the unsupported value
