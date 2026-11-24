# Design

## Context

See proposal.md. `closeOpen(tag)` walks the stack from the top and pops everything down to the first matching element, without regard to containers that start a new scope.

## Goals / Non-Goals

**Goals:**

- Nested tables and lists keep their structure.
- No change for flat documents.

**Non-Goals:**

- A full HTML5 tree construction algorithm with insertion modes.
- Foster parenting changes.

## Decisions

### 1. A scoped close with boundaries

`closeOpenIn(tag string, boundaries ...string)` scans from the top: if it meets a boundary before the tag, it returns false and leaves the stack alone; otherwise it pops down to and including the tag, exactly like `closeOpen`.

Mapping:

| Start tag | Tag closed | Boundaries |
|---|---|---|
| `li` | `li` | `ol`, `ul` |
| `td`, `th` | `td`, `th` | `table` |
| `tr` | `td`, `th`, `tr` | `table` |
| `thead`, `tbody`, `tfoot` | cells, rows, groups | `table` |
| `dt`, `dd` | `dt`, `dd` | `dl` |
| `option` | `option` | `select` |
| paragraph-closing tags | `p` | unchanged |

*Why boundaries rather than a full insertion-mode machine*: the profile supports a small HTML subset; a boundary table expresses the scoping rule directly and is easy to test.

### 2. Explicit end tags stay unscoped

`</td>`, `</li>` and friends still close the nearest matching element regardless of boundaries, because the author asked for it explicitly and browsers do the same.

## Risks / Trade-offs

- **A stray `<td>` outside a table now stays open** instead of closing something above; the forgiving builder already treats such elements as blocks, so the visible result is unchanged.
- **Paragraph closing** is left as is; it is a separate rule and the review does not report it.
