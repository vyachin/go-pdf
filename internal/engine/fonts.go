package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/asset"
	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/profile"
	"github.com/vyachin/go-pdf/internal/text"
)

// registeredFace is a font loaded for the document.
type registeredFace struct {
	family string
	weight int
	italic bool
	font   *font.Font
}

// fontRegistry resolves the font family lists of computed styles.
type fontRegistry struct {
	faces    []*registeredFace
	fallback *font.Font
	// fallbackFaces is the lazily loaded system family: the first available
	// DefaultFallbackFonts entry plus its bold and italic siblings.
	fallbackFaces []*registeredFace
	// systemErr is the terminal failure discovery hit, kept so every later
	// face request reports the same cause.
	systemErr error
	mu        sync.Mutex
	cache     map[string]*layout.FontFace
	// maxFontBytes and guard let the registry load a system fallback lazily,
	// only when a document really asks for a family it does not declare.
	maxFontBytes int
	guard        *budget.Guard
	// reg is the conversion's asset registry. It owns the confined reader, the
	// canonical identity and the conversion totals; parsed keys its cache with
	// the registry's identity, so one file is parsed once whatever named it.
	reg    *assets.Registry
	parsed map[string]*font.Font
	// maxTotalFontBytes is passed to every read so the registry enforces the
	// conversion total before a file is opened.
	maxTotalFontBytes int
	// loadFont is the font-parser entry point. It is font.LoadWithOptions
	// unless a test injects a counting wrapper for this registry instance; the
	// seam is per registry, so no global state or public API is involved.
	loadFont   func([]byte, font.ParseOptions) (*font.Font, error)
	systemOnce sync.Once
}

func newFontRegistry(ctx context.Context, guard *budget.Guard, sheets []*css.Stylesheet, opts Options, reg *assets.Registry) (*fontRegistry, error) {
	return newFontRegistryWithLoader(ctx, guard, sheets, opts, reg, nil)
}

// newFontRegistryWithLoader is newFontRegistry with an instance-local parser
// entry point. A nil loader uses font.LoadWithOptions, so a production
// conversion is unchanged; a test passes a wrapper that counts actual parses.
func newFontRegistryWithLoader(ctx context.Context, guard *budget.Guard, sheets []*css.Stylesheet, opts Options, reg *assets.Registry, loadFont func([]byte, font.ParseOptions) (*font.Font, error)) (*fontRegistry, error) {
	limits := opts.Limits.resolved()
	if reg == nil {
		reg = assets.New(guard)
	}
	r := &fontRegistry{
		cache:             map[string]*layout.FontFace{},
		guard:             guard,
		reg:               reg,
		parsed:            map[string]*font.Font{},
		loadFont:          loadFont,
		maxFontBytes:      limits.MaxFontBytes,
		maxTotalFontBytes: limits.MaxTotalFontBytes,
	}
	if err := guard.Err(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	fontDir := opts.FontDir
	if !reg.IsSource() && fontDir == "" {
		// On the host filesystem an empty FontDir historically falls back to
		// BaseDir. A source names each directory explicitly and defaults both
		// to its root, so no hidden host path enters the resolution.
		fontDir = opts.BaseDir
	}
	// The root is opened eagerly, so a configured but unusable directory is
	// reported before any font is read; the registry keeps the handle for the
	// conversion and closes it once. A source root is a directory inside the
	// caller's fs.FS and is checked in preparation instead.
	if fontDir != "" && !reg.IsSource() {
		if _, err := reg.Root(fontDir); err != nil {
			return nil, fmt.Errorf("opening the font directory: %w", err)
		}
	}
	maxFontBytes := limits.MaxFontBytes
	maxTotalFontBytes := int64(limits.MaxTotalFontBytes)
	for _, rule := range css.RulesForPrint(sheets) {
		if rule.Kind != css.FontFaceRule {
			continue
		}
		if err := guard.Add(1); err != nil {
			return nil, err
		}
		var family, src string
		var srcPos diag.Position
		weight, italic := 400, false
		for _, d := range rule.Decls {
			switch d.Property {
			case "font-family":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-family", d.Value, 12000); err == nil && len(st.FontFamily) > 0 {
					family = strings.ToLower(st.FontFamily[0])
				}
			case "src":
				if source, err := profile.ParseFontFaceSrc(d.Value); err == nil {
					src = source.Path
					// The diagnostic points at the declaration the author can
					// fix, not at the document root.
					if d.Sheet != nil {
						pos := d.Sheet.Position(d.Offset)
						srcPos = diag.Position{File: pos.File, Line: pos.Line, Col: pos.Col}
					}
				}
			case "font-weight":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-weight", d.Value, 12000); err == nil {
					weight = st.FontWeight
				}
			case "font-style":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-style", d.Value, 12000); err == nil {
					italic = st.FontStyle == css.FontItalic
				}
			}
		}
		if family == "" || src == "" {
			continue
		}
		// The src is a URL path: the query, fragment, percent encoding and
		// surrounding whitespace are resolved before the file is named.
		ref, refErr := asset.Resolve(src)
		if refErr != nil {
			return nil, &fontSourceError{err: refErr, pos: srcPos}
		}
		if ref.Remote {
			return nil, &fontSourceError{err: fmt.Errorf("remote fonts are not supported: %s", src), pos: srcPos}
		}
		if ref.Data {
			return nil, &fontSourceError{err: fmt.Errorf("data: fonts are not supported: %s", src), pos: srcPos}
		}
		src = ref.Path
		// The registry resolves the reference inside the font root and returns
		// the bytes with their canonical identity; two spellings of one file
		// therefore share one read, one admission and one parse.
		rec, readErr := reg.Read(assets.Request{
			Kind:          assets.Font,
			Reference:     src,
			Dir:           fontDir,
			MaxBytes:      maxFontBytes,
			MaxTotalBytes: maxTotalFontBytes,
		})
		if readErr != nil {
			var assetErr *assets.Error
			if errors.As(readErr, &assetErr) && errors.Is(assetErr.Err, fsx.ErrOutsideRoot) {
				return nil, &fontSourceError{err: fmt.Errorf("font %q is outside the font directory", src), pos: srcPos}
			}
			if errors.As(readErr, &assetErr) && errors.Is(assetErr.Err, assets.ErrTooLarge) {
				return nil, &fontSourceError{err: fmt.Errorf("%w: font %q exceeds the limit of %d bytes", budget.ErrExceeded, src, maxFontBytes), pos: srcPos}
			}
			if errors.Is(readErr, budget.ErrExceeded) {
				return nil, readErr
			}
			return nil, &fontSourceError{err: fmt.Errorf("loading font %q: %w", src, readErr), pos: srcPos}
		}
		canonical := rec.Canonical
		f, ok := r.parsedFont(rec.Key)
		if !ok {
			// Parsing a font is bounded by MaxFontBytes; the charge still makes
			// several medium fonts spend the shared work budget.
			if err := guard.Add(uint64(len(rec.Bytes))/1024 + 1); err != nil {
				return nil, err
			}
			loaded, parseErr := r.parseFont(rec.Bytes, font.ParseOptions{Guard: guard})
			if parseErr != nil {
				// A font that does not parse must not consume the total a later
				// valid font needs.
				reg.Discard(rec)
				return nil, &fontSourceError{err: loadFontError(src, parseErr), pos: srcPos}
			}
			f = r.keepFont(rec.Key, loaded)
		}
		key := fmt.Sprintf("%s|%d|%v|%s", canonical, weight, italic, family)
		if seen[key] {
			continue
		}
		seen[key] = true
		r.faces = append(r.faces, &registeredFace{family: family, weight: weight, italic: italic, font: f})
	}
	if opts.FallbackFont != "" {
		if err := guard.Err(); err != nil {
			return nil, err
		}
		// The explicit fallback is caller-selected: on the host filesystem it
		// is read as an operating-system path outside the document root; on a
		// source it is a path inside the source, still read as the caller wrote
		// it. Only after the caller named it. A file the document already
		// declared is shared from the same registry record and the same parse.
		rec, readErr := reg.Read(assets.Request{
			Kind:          assets.Font,
			Reference:     opts.FallbackFont,
			Explicit:      true,
			MaxBytes:      maxFontBytes,
			MaxTotalBytes: maxTotalFontBytes,
		})
		if readErr != nil {
			var assetErr *assets.Error
			if errors.As(readErr, &assetErr) && errors.Is(assetErr.Err, assets.ErrTooLarge) {
				return nil, fmt.Errorf("%w: fallback font %q exceeds the limit of %d bytes", budget.ErrExceeded, opts.FallbackFont, maxFontBytes)
			}
			return nil, fmt.Errorf("loading fallback font: %w", readErr)
		}
		f, ok := r.parsedFont(rec.Key)
		if !ok {
			if err := guard.Add(uint64(len(rec.Bytes))/1024 + 1); err != nil {
				return nil, err
			}
			loaded, parseErr := r.parseFont(rec.Bytes, font.ParseOptions{Guard: guard})
			if parseErr != nil {
				reg.Discard(rec)
				if errors.Is(parseErr, font.ErrLimit) {
					return nil, fmt.Errorf("%w: fallback font %q: %v", budget.ErrExceeded, opts.FallbackFont, parseErr)
				}
				return nil, fmt.Errorf("loading fallback font: %w", parseErr)
			}
			f = r.keepFont(rec.Key, loaded)
		}
		r.fallback = f
	}
	return r, nil
}

// fontSourceError carries the position of the @font-face src declaration that
// named a font file, so font/unavailable points at the line the author can fix.
type fontSourceError struct {
	err error
	pos diag.Position
}

func (e *fontSourceError) Error() string { return e.err.Error() }
func (e *fontSourceError) Unwrap() error { return e.err }

// parseFont parses one font through the registry's entry point. The default is
// font.LoadWithOptions; an instance-local test wrapper counts actual parses so
// one parse can be proven independently of the shared *font.Font pointer.
func (r *fontRegistry) parseFont(data []byte, opts font.ParseOptions) (*font.Font, error) {
	if r.loadFont != nil {
		return r.loadFont(data, opts)
	}
	return font.LoadWithOptions(data, opts)
}

// parsedFont returns a font another reference already parsed under this
// identity.
func (r *fontRegistry) parsedFont(key string) (*font.Font, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.parsed[key]
	return f, ok
}

// keepFont records one parsed font under its identity and returns the record to
// use. When two references parse the same file concurrently, the first stored
// font wins so every caller shares one *Font.
func (r *fontRegistry) keepFont(key string, f *font.Font) *font.Font {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.parsed[key]; ok {
		return existing
	}
	r.parsed[key] = f
	return f
}

// loadFontError reports why a font file was refused. A conversion that exceeds
// a documented bound is a resource/limit diagnostic, the same as an oversized
// file, and names the font; anything else is an unavailable font, with the
// parser's own explanation of the format or the damage.
func loadFontError(src string, err error) error {
	if errors.Is(err, font.ErrLimit) {
		return fmt.Errorf("%w: font %q: %v", budget.ErrExceeded, src, err)
	}
	return fmt.Errorf("loading font %q: %w", src, err)
}

// Face implements layout.FontSet.
func (r *fontRegistry) Face(families []string, weight int, italic bool, size int32) (*layout.FontFace, error) {
	// The key quotes every family name: joining the list with a separator a
	// name could contain made ["a b"] and ["a", "b"] share one cache entry.
	key := fmt.Sprintf("%q|%d|%t|%d", families, weight, italic, size)
	r.mu.Lock()
	if face, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return face, nil
	}
	r.mu.Unlock()

	f, err := r.pick(families, weight, italic)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("font family %q is not available", strings.Join(families, ", "))
	}
	face := &layout.FontFace{
		Font:    f,
		Size:    size,
		Ascent:  text.Scale(int32(f.Ascent()), size, f.UnitsPerEm()),
		Descent: text.Scale(-int32(f.Descent()), size, f.UnitsPerEm()),
		Key:     key,
	}
	r.mu.Lock()
	r.cache[key] = face
	r.mu.Unlock()
	return face, nil
}

func (r *fontRegistry) pick(families []string, weight int, italic bool) (*font.Font, error) {
	for _, family := range families {
		name := strings.ToLower(strings.TrimSpace(family))
		if name == "serif" || name == "sans-serif" || name == "monospace" {
			continue
		}
		var candidates []*registeredFace
		for _, face := range r.faces {
			if face.family == name {
				candidates = append(candidates, face)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		if best := bestFace(candidates, weight, italic); best != nil {
			return best, nil
		}
	}
	// An explicit Options.FallbackFont is step two of the documented order and
	// always wins over a system font.
	if r.fallback != nil {
		return r.fallback, nil
	}
	// Step three: the first available system family, including the bold and
	// italic faces that sit next to its regular file. Without the siblings a
	// certificate is printed with a regular title where the source asks for
	// 700.
	system, err := r.systemFaces()
	if err != nil {
		// A limit or cancellation during discovery is terminal: it must not be
		// mistaken for a missing family or change the selected typography.
		return nil, err
	}
	if best := bestFace(system, weight, italic); best != nil {
		return best, nil
	}
	// Never guess: substituting the first declared face would print an
	// undeclared family in a font the document did not ask for.
	return nil, nil
}

// bestFace picks the closest weight and slant from a family.
func bestFace(candidates []*registeredFace, weight int, italic bool) *font.Font {
	var best *registeredFace
	bestRank := 0
	bestItalic := false
	for _, c := range candidates {
		rank := weightRank(c.weight, weight)
		italicMatch := c.italic == italic
		if best == nil || rank < bestRank || (rank == bestRank && italicMatch && !bestItalic) {
			best, bestRank, bestItalic = c, rank, italicMatch
		}
	}
	if best == nil {
		return nil
	}
	return best.font
}

// systemFaces loads the first available system family once, so a document
// without @font-face still renders while a declared but unrelated family is not
// silently replaced by another declaration.
func (r *fontRegistry) systemFaces() ([]*registeredFace, error) {
	// The load reads files through the asset registry, which takes the same
	// lock while it parses; it must therefore run outside r.mu, once.
	r.systemOnce.Do(func() {
		faces, err := r.loadSystemFamily()
		r.mu.Lock()
		r.fallbackFaces = faces
		r.systemErr = err
		r.mu.Unlock()
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fallbackFaces, r.systemErr
}

// weightRank orders an available weight against the requested one, following
// the CSS Fonts fallback order. A lower rank is a better match; the exact
// weight is ranked first and the order of @font-face rules never decides
// between two different weights.
func weightRank(available, target int) int {
	if available == target {
		return 0
	}
	switch {
	case target < 400:
		// Lighter weights first, descending, then heavier ascending.
		if available < target {
			return 1 + (400 - available)
		}
		return 1000 + (available - target)
	case target <= 500:
		// The target..500 band ascending, then lighter descending, then
		// heavier than 500 ascending.
		if available >= target && available <= 500 {
			return 1 + (available - target)
		}
		if available < target {
			return 500 + (target - available)
		}
		return 1000 + (available - 500)
	default:
		// Heavier weights first, ascending, then lighter descending.
		if available > target {
			return 1 + (available - target)
		}
		return 1000 + (target - available)
	}
}

// DefaultFallbackFonts are searched when no font is configured. They exist to
// keep the CLI usable on a developer machine; production deployments should
// ship a font (see docs/fonts.md).
var DefaultFallbackFonts = []string{
	"/System/Library/Fonts/Supplemental/Arial.ttf",
	"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
}

func (r *fontRegistry) loadSystemFamily() ([]*registeredFace, error) {
	if r.reg.IsSource() {
		// A source is authoritative for the whole conversion: the host system
		// font list is not consulted, so an embedded deployment never mixes a
		// source with a host file. The caller ships the font in the source and
		// declares it, or names it with FallbackFont.
		return nil, nil
	}
	if err := r.guard.Err(); err != nil {
		return nil, err
	}
	for _, path := range DefaultFallbackFonts {
		if err := r.guard.Err(); err != nil {
			return nil, err
		}
		regular, err := r.loadFontFile(path)
		if err != nil {
			return nil, err
		}
		if regular == nil {
			// The candidate is absent or unusable: the search may continue.
			continue
		}
		faces := []*registeredFace{{weight: 400, font: regular}}
		for _, variant := range variantPaths(path) {
			if err := r.guard.Err(); err != nil {
				return nil, err
			}
			f, err := r.loadFontFile(variant)
			if err != nil {
				// The regular face fit but its sibling crossed a documented
				// bound: the family is refused rather than silently restyled.
				return nil, err
			}
			if f == nil {
				continue
			}
			weight, italic := variantStyle(variant)
			faces = append(faces, &registeredFace{weight: weight, italic: italic, font: f})
		}
		return faces, nil
	}
	return nil, nil
}

// loadFontFile loads one system font, applying the per-file and aggregate font
// limits before the file is opened. A nil font with a nil error means the
// candidate is absent or unusable, so the search may continue. A non-nil error
// is terminal: a documented bound was refused or the context was cancelled.
func (r *fontRegistry) loadFontFile(path string) (*font.Font, error) {
	rec, err := r.reg.Read(assets.Request{
		Kind:          assets.Font,
		Reference:     path,
		Explicit:      true,
		MaxBytes:      r.maxFontBytes,
		MaxTotalBytes: int64(r.maxTotalFontBytes),
	})
	if err != nil {
		if isTerminalFontError(err) {
			return nil, err
		}
		return nil, nil
	}
	if f, ok := r.parsedFont(rec.Key); ok {
		return f, nil
	}
	// A system font is charged exactly like an @font-face file, so the budget
	// accounts for every font the conversion parses, whatever named it.
	if err := r.guard.Add(uint64(len(rec.Bytes))/1024 + 1); err != nil {
		r.reg.Discard(rec)
		return nil, err
	}
	f, err := r.parseFont(rec.Bytes, font.ParseOptions{Guard: r.guard})
	if err != nil {
		r.reg.Discard(rec)
		if errors.Is(err, font.ErrLimit) {
			return nil, fmt.Errorf("%w: %v", budget.ErrExceeded, err)
		}
		if isTerminalFontError(err) {
			return nil, err
		}
		return nil, nil
	}
	return r.keepFont(rec.Key, f), nil
}

// isTerminalFontError reports whether a fallback failure must end the
// conversion instead of letting discovery try the next candidate: a resource
// limit or a cancelled context. A corrupt or unreadable file is only an
// unavailable candidate.
func isTerminalFontError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, budget.ErrExceeded) {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// variantPaths lists the conventional bold and italic file names next to a
// regular face: "Arial Bold.ttf" and "DejaVuSans-Bold.ttf" styles.
func variantPaths(path string) []string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(filepath.Base(path), ext)
	base := strings.TrimSuffix(stem, "-Regular")
	var out []string
	for _, suffix := range []string{" Bold", "-Bold", " Italic", "-Italic", " Oblique", "-Oblique", " Bold Italic", "-BoldItalic", "-BoldOblique"} {
		out = append(out, filepath.Join(dir, base+suffix+ext))
	}
	return out
}

func variantStyle(path string) (int, bool) {
	lower := strings.ToLower(path)
	weight := 400
	if strings.Contains(lower, "bold") {
		weight = 700
	}
	italic := strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
	return weight, italic
}
