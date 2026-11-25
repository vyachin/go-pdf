# Browser certificate fixture

`certificate.html` is a browser-oriented certificate template that uses
flexbox, absolute positioning, CSS variables, `rem` units and `data:` URI
images. It was the acceptance fixture for the removed `ImportBrowser`
compatibility layer.

The file is kept byte for byte as a **rejection fixture**: converting or
validating it must fail with ordinary profile diagnostics and produce no PDF.
The separate profile-compatible certificate remains the supported visual
fixture.
