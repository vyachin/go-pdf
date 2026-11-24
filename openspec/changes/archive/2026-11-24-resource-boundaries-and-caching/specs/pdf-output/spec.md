# Spec Delta

## ADDED Requirements

### Requirement: Page resources list only images the page draws

A page's resource dictionary SHALL list only the images that page actually draws. Images used on other pages MUST NOT appear in it. The order of the listed images MUST remain deterministic.

#### Scenario: An image used on one page
- **WHEN** a document of two pages draws an image only on the first page
- **THEN** the second page's resources contain no image entries

#### Scenario: Two images on the same page
- **WHEN** a page draws two different images
- **THEN** both appear in that page's resources in document order

#### Scenario: Output stays deterministic
- **WHEN** the same document is serialised repeatedly
- **THEN** every run produces identical bytes

### Requirement: A repeated image is embedded once

When the same image source is used several times in a document, the engine SHALL decode it once and embed a single image object.

#### Scenario: The same image twice
- **WHEN** a document references the same image file in two places
- **THEN** the PDF contains one image object for it
- **AND** both places draw that object
