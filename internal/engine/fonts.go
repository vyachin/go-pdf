package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
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
	mu            sync.Mutex
	cache         map[string]*layout.FontFace
	// maxFontBytes and guard let the registry load a system fallback lazily,
	// only when a document really asks for a family it does not declare.
	maxFontBytes int
	guard        *budget.Guard
	systemTried  bool
}

func newFontRegistry(ctx context.Context, guard *budget.Guard, sheets []*css.Stylesheet, opts Options) (*fontRegistry, error) {
	r := &fontRegistry{cache: map[string]*layout.FontFace{}, guard: guard, maxFontBytes: opts.Limits.resolved().MaxFontBytes}
	if err := guard.Err(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	parsed := map[string]*font.Font{}
	fontDir := opts.FontDir
	if fontDir == "" {
		fontDir = opts.BaseDir
	}
	fontRoot, fontRootErr := fsx.Open(fontDir)
	if fontRootErr != nil {
		return nil, fmt.Errorf("opening the font directory: %w", fontRootErr)
	}
	defer func() { _ = fontRoot.Close() }()
	maxFontBytes := opts.Limits.resolved().MaxFontBytes
	for _, rule := range css.RulesForPrint(sheets) {
		if rule.Kind != css.FontFaceRule {
			continue
		}
		if err := guard.Add(1); err != nil {
			return nil, err
		}
		var family, src string
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
		path := resolvePath(src, opts.FontDir, opts.BaseDir)
		// The file is parsed once; every family alias that names it still gets
		// its own registration.
		f, ok := parsed[path]
		if !ok {
			data, err := fontRoot.ReadLimit(path, maxFontBytes)
			if err != nil {
				if errors.Is(err, fsx.ErrOutsideRoot) {
					return nil, fmt.Errorf("font %q is outside the font directory", src)
				}
				if errors.Is(err, fsx.ErrTooLarge) {
					return nil, fmt.Errorf("%w: font %q exceeds the limit of %d bytes", budget.ErrExceeded, src, maxFontBytes)
				}
				return nil, fmt.Errorf("loading font %q: %w", src, err)
			}
			// Parsing a font is bounded by MaxFontBytes; the charge still makes
			// several medium fonts spend the shared work budget.
			if err := guard.Add(uint64(len(data))/1024 + 1); err != nil {
				return nil, err
			}
			f, err = font.LoadWithOptions(data, font.ParseOptions{Guard: guard})
			if err != nil {
				return nil, loadFontError(src, err)
			}
			parsed[path] = f
		}
		key := fmt.Sprintf("%s|%d|%v|%s", path, weight, italic, family)
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
		data, err := fsx.ReadFileLimit(opts.FallbackFont, maxFontBytes)
		if err != nil {
			if errors.Is(err, fsx.ErrTooLarge) {
				return nil, fmt.Errorf("%w: fallback font %q exceeds the limit of %d bytes", budget.ErrExceeded, opts.FallbackFont, maxFontBytes)
			}
			return nil, fmt.Errorf("loading fallback font: %w", err)
		}
		if err := guard.Add(uint64(len(data))/1024 + 1); err != nil {
			return nil, err
		}
		f, err := font.LoadWithOptions(data, font.ParseOptions{Guard: guard})
		if err != nil {
			if errors.Is(err, font.ErrLimit) {
				return nil, fmt.Errorf("%w: fallback font %q: %v", budget.ErrExceeded, opts.FallbackFont, err)
			}
			return nil, fmt.Errorf("loading fallback font: %w", err)
		}
		r.fallback = f
	}
	return r, nil
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
	key := fmt.Sprintf("%v|%d|%v|%d", families, weight, italic, size)
	r.mu.Lock()
	if face, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return face, nil
	}
	r.mu.Unlock()

	f := r.pick(families, weight, italic)
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

func (r *fontRegistry) pick(families []string, weight int, italic bool) *font.Font {
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
			return best
		}
	}
	// An explicit Options.FallbackFont is step two of the documented order and
	// always wins over a system font.
	if r.fallback != nil {
		return r.fallback
	}
	// Step three: the first available system family, including the bold and
	// italic faces that sit next to its regular file. Without the siblings a
	// certificate is printed with a regular title where the source asks for
	// 700.
	if best := bestFace(r.systemFaces(), weight, italic); best != nil {
		return best
	}
	// Never guess: substituting the first declared face would print an
	// undeclared family in a font the document did not ask for.
	return nil
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
func (r *fontRegistry) systemFaces() []*registeredFace {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.systemTried {
		return r.fallbackFaces
	}
	r.systemTried = true
	r.fallbackFaces = loadSystemFamily(r.maxFontBytes, r.guard)
	return r.fallbackFaces
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

func resolvePath(path, fontDir, baseDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if fontDir != "" {
		return filepath.Join(fontDir, path)
	}
	if baseDir != "" {
		return filepath.Join(baseDir, path)
	}
	return path
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

func loadSystemFamily(maxBytes int, guard *budget.Guard) []*registeredFace {
	if guard.Err() != nil {
		return nil
	}
	// PDFTEST_FONT is an explicit override: use exactly that face and do not
	// guess variants next to it, so tests stay reproducible.
	if env := os.Getenv("PDFTEST_FONT"); env != "" {
		if f := loadFontFile(env, maxBytes, guard); f != nil {
			return []*registeredFace{{weight: 400, font: f}}
		}
	}
	for _, path := range DefaultFallbackFonts {
		if guard.Err() != nil {
			return nil
		}
		regular := loadFontFile(path, maxBytes, guard)
		if regular == nil {
			continue
		}
		faces := []*registeredFace{{weight: 400, font: regular}}
		for _, variant := range variantPaths(path) {
			f := loadFontFile(variant, maxBytes, guard)
			if f == nil {
				continue
			}
			weight, italic := variantStyle(variant)
			faces = append(faces, &registeredFace{weight: weight, italic: italic, font: f})
		}
		return faces
	}
	return nil
}

func loadFontFile(path string, maxBytes int, guard *budget.Guard) *font.Font {
	data, err := fsx.ReadFileLimit(path, maxBytes)
	if err != nil {
		return nil
	}
	f, err := font.LoadWithOptions(data, font.ParseOptions{Guard: guard})
	if err != nil {
		return nil
	}
	return f
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
