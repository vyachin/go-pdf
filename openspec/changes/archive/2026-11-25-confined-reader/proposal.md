# Proposal

## Why

Three follow-ups around the confined reader introduced with `asset-boundaries`:

- `ReadWithin` returns `ErrOutsideRoot` for any open failure except "file not found", so a permission error is reported as "path escapes the base directory". The behaviour is safe but the diagnostic misleads the operator.
- the root directory is resolved and reopened for every asset, so a document with many images pays `EvalSymlinks` and `OpenRoot` per file;
- the helper lives in `internal/profile` although it is a filesystem primitive used by image and font loaders.

## What Changes

- A new package `internal/fsx` owns the confined reader: it resolves a root once, keeps the `*os.Root` handle, and provides `Read` for paths inside it.
- `Read` SHALL distinguish a path escaping the root from any other open error and return the original error otherwise.
- `profile`, the image loader and the font registry SHALL each create one root per conversion and reuse it for every asset of that class.

Out of scope: budgets and cancellation (ENG-02), table and cascade fixes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the asset boundary requirement gains the diagnostic distinction between an escaping path and an unreadable one.

## Impact

- **Code**: new `internal/fsx`, `internal/profile/styles.go`, `internal/engine/{images,fonts}.go`.
- **Behaviour**: unchanged placement and rendering; a permission failure now reports the real cause.
- **Performance**: one root resolution per asset class per conversion instead of one per file.
