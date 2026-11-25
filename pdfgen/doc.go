// Package pdfgen converts an HTML document into a PDF.
//
// It is a library for server-side business documents — invoices, acts,
// contracts, reports — where the same template must reproduce the same file
// today and in a year. The command in cmd/pdfgen is the reference example of
// how to call it; its source is the shortest complete program that uses this
// package.
//
// # Guarantees
//
//   - Deterministic output. The same engine build, HTML bytes, options and
//     resolved asset bytes — linked stylesheets, images and selected font bytes
//     — produce a byte-identical PDF. [Options].Date defaults to a fixed
//     constant rather than the system clock, and the document identifier is a
//     hash of the content. A file edited in place is a changed input, and
//     system-font fallback is environment-dependent, so a reproducible
//     deployment selects [Options].FallbackFont (or an @font-face source) and
//     keeps its stylesheets and images immutable.
//   - No writes. [Convert] and [Validate] create, modify and delete no files.
//     They read only the assets the document references, inside the configured
//     roots, and the PDF is returned to the caller, who decides where it goes.
//   - No panics. Any byte sequence is answered with a result or an error, never
//     with a crash.
//   - Concurrency. The package keeps no mutable global state, so independent
//     calls may run in parallel and agree with sequential ones.
//   - Bounded work. [Options].Limits caps the input, page count, image pixels,
//     font bytes, nesting depth and total work; a document over a limit fails
//     with a resource/limit diagnostic before the expensive allocation.
//
// # What it does not do
//
// The package renders a closed CSS profile, not a browser: there is no flexbox
// or grid layout, no absolute positioning, no JavaScript and no network access.
// Images, stylesheets and fonts are local files. Every document is validated
// against the closed profile; there is no lowering pass.
//
// # Defaults
//
// The zero [Options] value is a complete request: A4, a 20 mm margin, the fixed
// creation date and the default limits. Relative image and font paths resolve
// against [Options].BaseDir and [Options].FontDir; when both are empty, the
// process working directory is used.
//
// # Diagnostics
//
// An unusable document produces an [Error] whose [Diagnostics] carry a stable
// Code, a 1-based source position and the element's path. [Convert] and
// [Validate] report every problem they find in one pass, in a deterministic
// order, and [Validate] renders no PDF.
//
// The supported elements, properties and limits are listed in docs/profile.md.
// The licence is in LICENSE.
package pdfgen
