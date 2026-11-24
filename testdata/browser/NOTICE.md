# Browser certificate fixture

`certificate.html` is the browser-oriented certificate template that the
browser import (`engine.Options.ImportBrowser`) must accept without edits: it
uses flexbox, absolute positioning, CSS variables, `rem` units and `data:` URI
images. The images are embedded in the document, so the fixture is
self-contained.

The acceptance test is `TestBrowserCertificateImport` in
`internal/engine/browser_import_test.go`.
