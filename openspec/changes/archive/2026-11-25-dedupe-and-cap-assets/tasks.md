# Tasks

## 1. Canonical identity

- [x] 1.1 Resolve every image reference to a canonical absolute path (after symlinks) and key the cache by it — verify: `a.png` and `./a.png` produce one image object.
- [x] 1.2 Key PDF image objects by content hash instead of `*ImageData` pointer — verify: the 50-spelling document embeds one object and its PDF stays under the asset size.
- [x] 1.3 Cache `data:` URIs by their bytes — verify: a URI used by an `<img>` and a background produces one object and one decode.

## 2. Totals

- [x] 2.1 Add total image bytes, image pixels and font bytes to `Limits` with documented defaults — verify: zero values resolve to the defaults and `go doc` shows them.
- [x] 2.2 Charge the totals before reading and decoding, returning `resource/limit` — verify: a document over a total stops before the offending asset is opened.
- [x] 2.3 Add the 50-spelling and repeated-data-URI regressions — verify: each fails before the fix and passes after; a normal invoice stays byte-identical.

## 3. Documentation

- [x] 3.1 Document the totals in `docs/profile.md` and the `Limits` doc comments — verify: the documented defaults match the resolved values.
