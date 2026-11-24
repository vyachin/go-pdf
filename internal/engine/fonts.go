package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/font"
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

func newFontRegistry(sheets []*css.Stylesheet, opts Options) (*fontRegistry, error) {
	r := &fontRegistry{cache: map[string]*layout.FontFace{}}
	seen := map[string]bool{}
	for _, rule := range css.RulesForPrint(sheets) {
		if rule.Kind != css.FontFaceRule {
			continue
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
		key := fmt.Sprintf("%s|%d|%v", path, weight, italic)
		if seen[key] {
			continue
		}
		seen[key] = true
		root := opts.FontDir
		if root == "" {
			root = opts.BaseDir
		}
		data, err := profile.ReadWithin(root, path)
		if err != nil {
			if errors.Is(err, profile.ErrOutsideRoot) {
				return nil, fmt.Errorf("font %q is outside the font directory", src)
			}
			return nil, fmt.Errorf("loading font %q: %w", src, err)
		}
		f, err := font.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("loading font %q: %w", src, err)
		}
		r.faces = append(r.faces, &registeredFace{family: family, weight: weight, italic: italic, font: f})
	}
	if opts.FallbackFont != "" {
		data, err := os.ReadFile(opts.FallbackFont)
		if err != nil {
			return nil, fmt.Errorf("loading fallback font: %w", err)
		}
		f, err := font.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("loading fallback font: %w", err)
		}
		r.fallback = f
	}
	if r.fallback == nil && len(r.faces) == 0 {
		if f := loadSystemFallback(); f != nil {
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
		best := candidates[0]
		bestScore := -1
		for _, c := range candidates {
			score := 0
			if c.weight == weight {
				score += 2
			} else if (weight >= 600 && c.weight >= 600) || (weight < 600 && c.weight < 600) {
				score++
			}
			if c.italic == italic {
				score++
			}
			if score > bestScore {
				best, bestScore = c, score
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

func loadSystemFallback() *font.Font {
	candidates := append([]string{}, DefaultFallbackFonts...)
	if env := os.Getenv("PDFTEST_FONT"); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
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
