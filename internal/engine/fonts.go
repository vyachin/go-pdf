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
	mu       sync.Mutex
	cache    map[string]*layout.FontFace
}

func newFontRegistry(ctx context.Context, guard *budget.Guard, sheets []*css.Stylesheet, opts Options) (*fontRegistry, error) {
	r := &fontRegistry{cache: map[string]*layout.FontFace{}}
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
	defer fontRoot.Close()
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
			f, err = font.Parse(data)
			if err != nil {
				return nil, fmt.Errorf("loading font %q: %w", src, err)
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
		f, err := font.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("loading fallback font: %w", err)
		}
		r.fallback = f
	}
	if r.fallback == nil && len(r.faces) == 0 {
		if f := loadSystemFallback(maxFontBytes, guard); f != nil {
			r.fallback = f
		}
	}
	if r.fallback == nil && len(r.faces) == 0 {
		return nil, fmt.Errorf("no fonts available: declare @font-face or pass a fallback font")
	}
	return r, nil
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
		return best.font
	}
	if r.fallback != nil {
		return r.fallback
	}
	if len(r.faces) > 0 {
		return r.faces[0].font
	}
	return nil
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

func loadSystemFallback(maxBytes int, guard *budget.Guard) *font.Font {
	candidates := append([]string{}, DefaultFallbackFonts...)
	if env := os.Getenv("PDFTEST_FONT"); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	for _, path := range candidates {
		if guard.Err() != nil {
			return nil
		}
		data, err := fsx.ReadFileLimit(path, maxBytes)
		if err != nil {
			continue
		}
		f, err := font.Parse(data)
		if err != nil {
			continue
		}
		return f
	}
	return nil
}
