# Design

## Context

`imageLoader` caches by `src` string and the writer keys images by `*ImageData`. The
filesystem layer already resolves a path through `fsx.Root`, which knows the canonical
form. Limits are per file, and `Options` is additive-only.

## Goals / Non-Goals

**Goals:** one decode and one PDF object per real file; aggregate caps that stop a
document before it allocates hundreds of megabytes.

**Non-Goals:** deduplicating images with equal bytes that live in different files
(content-hash dedupe is a second level, not a substitute for path identity); caching
across conversions.

## Decisions

- Identity is the canonical absolute path after `EvalSymlinks`, computed once before
  the cache lookup. For `data:` URIs the identity is the SHA-256 of the decoded bytes.
- Keep the existing per-file limits; add `MaxTotalImageBytes`, `MaxTotalImagePixels`
  and `MaxTotalFontBytes` to `Limits` with defaults above a normal invoice (64 MiB,
  80 MP, 32 MiB).
- Count totals when an asset is admitted, before reading, so a rejected asset is never
  opened; a hash-deduplicated asset is counted once.
- Deduplicate the writer's images by the same identity so two `ImageData` pointers for
  one file still produce one stream.

## Risks / Trade-offs

- Symlink resolution costs a stat per asset → cache the resolved path with the image.
- Aggregate defaults could reject a legitimate catalogue → document them and make the
  fields additive so a caller can raise them without a breaking change.
