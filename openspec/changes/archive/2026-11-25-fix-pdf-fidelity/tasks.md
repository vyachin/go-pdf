# Tasks

## 1. Writer fidelity

- [x] 1.1 Add subset tags and real `CapHeight`/`ItalicAngle` accessors and emit them in the descriptor — verify: `pdffonts` reports `sub yes` and the italic PDF reports the font's angle.
- [x] 1.2 Reject invalid UTF-8 metadata with `options/title` (and the author/subject equivalents) — verify: `pdfinfo` round-trips the valid case and the invalid case returns no PDF.
- [x] 1.3 Return an error for a nil font and `resource/limit` for more than 65,535 CID pairs — verify: both unit tests pass without a panic.
- [x] 1.4 Normalise outline levels in the build and the comparison — verify: the zero-level case produces siblings.

## 2. Raster and tooling

- [x] 2.1 Intersect the raster destination with the scaled `ImageOp.Clip` — verify: the covered-background PNG has page background outside the box and the PDF is unchanged.
- [x] 2.2 Drive `pdfdebug` pages and streams from the page tree and sort the ids — verify: the JPEG/TJ and font/TJ fixtures report one page, and 100 runs keep the order.
- [x] 2.3 Regenerate the goldens affected by subset names and descriptor values — verify: the render and document snapshots pass on regenerated files.

## 3. Tests and docs

- [x] 3.1 Add pdfcpu strict, `pdffonts`, non-BMP and clip assertions — verify: each fails on the parent commit.
- [x] 3.2 Update `docs/fonts.md` for subset naming and descriptor fields — verify: the documented values match the emitted ones.
