# Tasks

## 1. Shared confined reader

- [x] 1.1 Extract the root-confined read into `profile.ReadWithin` with an `ErrOutsideRoot` sentinel and make the CSS loader use it — verification: existing CSS path tests keep their diagnostics, symlink escape still fails
- [x] 1.2 Keep the no-root behaviour: an empty root reads directly — verification: existing tests that run without a base directory pass

## 2. Images inside BaseDir

- [x] 2.1 Open images through `ReadWithin(l.baseDir, path)` — verification: a relative image inside `BaseDir` loads and is embedded
- [x] 2.2 Reject absolute paths outside the root and symlinks that escape — verification: two tests fail conversion with a diagnostic and leave the outside file unread
- [x] 2.3 Accept symlinks that stay inside the root — verification: an internal symlink loads the image

## 3. Fonts inside the font root

- [x] 3.1 Open `@font-face` files through `ReadWithin(FontDir or BaseDir, path)` — verification: a font inside the root loads
- [x] 3.2 Reject a font path outside the font root — verification: test expects `font/unavailable`
- [x] 3.3 Keep the fallback font and system fonts trusted — verification: conversion with an absolute fallback path outside the roots still works

## 4. Table header repetition refactor

- [x] 4.1 Move the repetition into `paintRepeatedHeaders(f, t) bool` and use its result in `placeTable` — verification: the two-row `thead` test and the corpus pass

## 5. Acceptance

- [x] 5.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate asset-boundaries` — verification: everything green
- [x] 5.2 Re-check the boundary cases by hand: absolute image outside, escaping symlink, internal symlink, font outside the root — verification: each produces the documented outcome
