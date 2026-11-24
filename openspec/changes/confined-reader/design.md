# Design

## Context

See proposal.md. `ReadWithin` resolves the root and opens it on every call; the failure mapping is:

```go
f, err := root.Open(rel)
if err != nil {
	if os.IsNotExist(err) { return nil, err }
	return nil, ErrOutsideRoot     // everything else, including permissions
}
```

## Goals / Non-Goals

**Goals:**

- One owner of the boundary rule, used by stylesheets, images and fonts.
- Precise diagnostics: escape versus unreadable versus missing.
- One root handle per asset class per conversion.

**Non-Goals:**

- Caching file contents across conversions.
- Budgets and cancellation.

## Decisions

### 1. `fsx.Root` holds the resolved directory and the handle

```go
type Root struct { dir string; handle *os.Root }
func Open(dir string) (*Root, error)          // resolves dir once
func (r *Root) Read(path string) ([]byte, error)
func (r *Root) Close() error
```

`Read` resolves the target with `EvalSymlinks`, checks containment with `filepath.Rel`, then opens the relative path through the handle. A zero `Root` (no directory) reads directly, which is the existing "no boundary configured" contract.

*Why keep `EvalSymlinks`*: it lets an absolute symlink that points inside the root work, and keeps the lexical rejection of `..` precise; the `os.Root` open then covers the race.

### 2. Escape detection, not blanket mapping

`os.Root.Open` reports an escaping path as a `*os.PathError` whose message contains "escapes from parent"; there is no exported sentinel. `Read` maps that case to `ErrOutsideRoot` and returns every other error unchanged, so permission failures surface as themselves.

*Why not treat all errors as escapes*: the fail-closed behaviour was right, the message was wrong; conflating the two hides real filesystem problems from the operator.

### 3. One root per asset class per conversion

`profile.CollectStylesheets` opens `BaseDir` once; the image loader opens `BaseDir` lazily on first use; the font registry opens `FontDir` (else `BaseDir`) once before iterating `@font-face` rules. Each closes its root when done.

*Why per class and not one shared root*: images and styles share `BaseDir`, fonts may use `FontDir`; sharing one object across packages would need an owner and a lifetime, which is more machinery than the saving justifies.

## Risks / Trade-offs

- **Root lifetime**: forgetting `Close` leaks a descriptor per conversion; the loaders close with `defer` at their call site.
- **Open failure timing** moves from the first asset to root creation; both are reported as the same diagnostic code.
- **New package** adds one import path in three places; it removes a filesystem primitive from a rules package.
