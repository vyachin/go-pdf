# Spec Delta

## ADDED Requirements

### Requirement: Background images cover their box

The engine SHALL paint a box's background image before the box's content, scaled so that the image covers the box and centered, with the overflow divided equally between opposite edges. A box without a background image SHALL lay out as before.

#### Scenario: A photograph behind a heading
- **WHEN** a full-page cell has a photograph as its background and a heading as its content
- **THEN** the photograph covers the whole page and the heading is painted over it

#### Scenario: A cropped cover
- **WHEN** a 2:3 image covers an A4 box
- **THEN** the image is scaled to the box width and the vertical overflow is split equally between the top and the bottom

#### Scenario: Text without a background image
- **WHEN** the box declares only `background-color`
- **THEN** no image operation is added to the page

### Requirement: Background images are clipped to their box

The engine SHALL clip a background image to the box it belongs to, so that no pixel outside the box receives the image, and the clip SHALL NOT affect any other operation on the page.

#### Scenario: Overflow beyond the box
- **WHEN** the covered image is wider or taller than the box
- **THEN** the produced page has no image pixel outside the box edges

#### Scenario: Neighbouring content stays unclipped
- **WHEN** a page contains the covered box and other content
- **THEN** that other content is painted in full, unaffected by the background's clip
