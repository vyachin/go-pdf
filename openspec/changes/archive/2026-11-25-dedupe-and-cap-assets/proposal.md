# Proposal

## Why

The image cache is keyed by the raw `src` string, while PDF objects are deduplicated
by `*layout.ImageData` pointer. One 1.96 MB PNG referenced through 50 spellings
(`a.png`, `./a.png`, `sub/../a.png`, …) inside 3.3 KB of HTML produces a 98 MB PDF,
100 image objects and about 200 MB of heap while every per-file limit passes: the
same file is read, decoded and embedded 50 times. `data:` URIs are not cached at all,
and nothing bounds the total bytes or pixels of the assets one conversion may read.

## What Changes

- Asset identity SHALL be the canonical resolved file (or the content hash), not the
  spelling of `src`: every spelling of one file MUST share one cache entry and one
  PDF object.
- A repeated `data:` URI SHALL be decoded once and embedded once.
- `Limits` SHALL gain aggregate per-document caps for image bytes, image pixels and
  font bytes; an asset that would cross a cap MUST fail with `resource/limit` before
  it is read or decoded.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: image identity is the file, not the URL spelling; repeated data URIs
  are embedded once.
- `library-api`: limits bound the totals of one conversion, not only each file.

## Impact

- **Code**: `internal/engine/images.go`, `internal/engine/fonts.go`,
  `internal/engine/convert.go`, `internal/pdf/writer.go`.
- **Output**: a repeated asset adds one object regardless of spelling; hostile
  documents fail with `resource/limit`.
- **Tests**: the 50-spelling measurement and a repeated data URI become regressions;
  a normal invoice with a repeated logo stays byte-identical.
