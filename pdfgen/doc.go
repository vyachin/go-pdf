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
//     hash of the content. A file edited in place is a changed input, so a
//     reproducible deployment keeps its stylesheets and images immutable and
//     selects the font bytes with an @font-face source or an explicit
//     [Options].FallbackFont/[Options].FallbackFonts entry.
//   - No writes. [Convert], [ConvertTo] and [Validate] create, modify and delete
//     no files. They read only the assets the document references, inside the
//     configured roots or the caller-supplied [Options].Source, and the PDF is
//     returned to the caller, who decides where it goes. [ConvertTo] writes only
//     to the io.Writer it is given.
//   - Streaming output. [ConvertTo] runs the same conversion as [Convert] and
//     writes the PDF to an io.Writer as it is serialised, holding at most one
//     page's content stream instead of the complete document. Its [Result].PDF
//     is nil and the bytes it writes are identical to Convert's. A write error
//     or a cancelled context can leave a prefix of the PDF in the writer, and
//     the caller must discard it.
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
// Images, stylesheets and fonts are local files or, when [Options].Source is
// set, objects in that file system. Every document is validated against the
// closed profile; there is no lowering pass.
//
// # Defaults
//
// The zero [Options] value is a complete request: A4, a 20 mm margin, the fixed
// creation date and the default limits. Relative image and font paths resolve
// against [Options].BaseDir and [Options].FontDir; when both are empty, the
// process working directory is used.
//
// # Fonts
//
// A family resolves in one documented order: a declared @font-face family,
// then [Options].FallbackFont, then the ordered [Options].FallbackFonts list,
// then the font/unavailable diagnostic. The package reads no installed font of
// its own: a document that declares no usable family and passes no fallback
// fails instead of rendering host-dependent glyphs. [SystemFallbackFonts]
// returns the conventional platform paths for callers that explicitly opt into
// that non-reproducible behaviour.
//
// # Asset sources
//
// [Options].Source accepts any fs.FS — an embed.FS, an os.DirFS, a
// fstest.MapFS or a subtree made with fs.Sub. When it is set, it replaces the
// host filesystem for the whole conversion: every document-referenced image,
// stylesheet and @font-face source, and
// [Options].FallbackFont/[Options].FallbackFonts, is read through it.
// [Options].BaseDir and [Options].FontDir then name directories inside the
// source and default to its root, and no host file or installed host font is
// consulted. The two authorities are never mixed: a source path is relative,
// must stay inside the source and is rejected with the asset's load diagnostic
// otherwise. A nil Source reads from the host filesystem exactly as before.
//
// # Diagnostics
//
// An unusable document produces an [Error] whose [Diagnostics] carry a stable
// Code, a 1-based source position and the element's path. [Convert],
// [ConvertTo] and [Validate] report every problem they find in one pass, in a
// deterministic order, and [Validate] renders no PDF.
//
// The supported elements, properties and limits are listed in docs/profile.md.
// The licence is in LICENSE.
package pdfgen
