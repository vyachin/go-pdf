# Spec Delta

## ADDED Requirements

### Requirement: RenderPNG is a contained debug preview (RV5-51)

`Options.RenderPNG` SHALL be documented as a debug-only preview with its exact
contract — first page only, 96 dpi, bounded by `Limits.MaxRasterPixels` — and
MUST be marked deprecated. A PNG encoding failure MUST produce a
`raster/render` error diagnostic, never a silent nil `Result.PNG`, and enabling
the preview MUST NOT change the PDF bytes. Removing the field SHALL require a
major release.

#### Scenario: PNG encoding fails

- **WHEN** the preview raster is produced but PNG encoding returns an error
- **THEN** the conversion reports `raster/render` naming the document
- **AND** `Convert` returns an error
- **AND** `Result.PNG` is nil

#### Scenario: The preview does not change the document

- **WHEN** the same input is converted with `RenderPNG` false and true
- **THEN** the two PDF byte slices are identical
- **AND** only the requested conversion has a non-nil `Result.PNG`

#### Scenario: The preview is not requested

- **WHEN** `RenderPNG` is false
- **THEN** no raster work is done
- **AND** `Result.PNG` is nil and no `raster/render` diagnostic appears

#### Scenario: The contract and deprecation are documented

- **WHEN** the godoc of `Options.RenderPNG` is read
- **THEN** it states first page only, 96 dpi, `MaxRasterPixels` and a
  `Deprecated:` paragraph naming the major release that may remove it
- **AND** the engine documentation no longer claims a CLI debug flag uses it
