# Tasks

## 1. Marks and separators

- [x] 1.1 Implement MarkBase/MarkLig/MarkMark positioning for separate marks, or a diagnostic when absent — verify: the "сло́во" fixture draws the acute over "о" and extraction matches.
- [x] 1.2 Map `\f`, U+2028 and U+2029 to line breaks in `Split` for both preformatted and normal text — verify: the three separator fixtures render two lines.

## 2. CFF and composites

- [x] 2.1 Reject CFF deltas outside int16 in the converter — verify: the 32767.99998 fixture fails with a diagnostic.
- [x] 2.2 Apply `SCALED_COMPONENT_OFFSET` in composite placement — verify: the scaled-offset fixture yields the transformed point.
- [x] 2.3 Treat a missing `Private` DICT as empty — verify: the minimal CFF fixture loads and draws glyph 0.

## 3. Tests and documentation

- [x] 3.1 Add all five fixtures as regressions — verify: each fails before its fix.
- [x] 3.2 Update `docs/fonts.md` for mark attachment, separators and CFF strictness — verify: the documented behaviour matches the scenarios.
