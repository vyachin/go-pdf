package raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/text"
)

func newPage(w, h int32, ops ...layout.Op) *layout.Page {
	return &layout.Page{
		Number: 1,
		Size:   css.PageSize{Width: w, Height: h},
		Ops:    ops,
	}
}

func countDark(img *image.RGBA) int {
	n := 0
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r < 0x8000 {
				n++
			}
		}
	}
	return n
}

func TestFillRectArea(t *testing.T) {
	// 20mm x 10mm black rectangle at 72 dpi: 1mm is about 2.835 px.
	page := newPage(100000, 100000, layout.Op{
		Kind:  layout.FillOpKind,
		Rect:  layout.Rect{X: 0, Y: 0, W: 20000, H: 10000},
		Color: css.Color{Set: true},
	})
	img, err := Render(page, Options{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	dark := countDark(img)
	// 20pt x 10pt at 72 dpi is 20 x 10 pixels.
	want := 200
	if dark < want-2 || dark > want+2 {
		t.Fatalf("dark pixels = %d, want about %d", dark, want)
	}
}

func TestGlyphRingIsHollow(t *testing.T) {
	f := loadFont(t)
	face := &layout.FontFace{
		Font:    f,
		Size:    200000, // 200pt, large enough for a stable test
		Ascent:  text.Scale(int32(f.Ascent()), 200000, f.UnitsPerEm()),
		Descent: text.Scale(-int32(f.Descent()), 200000, f.UnitsPerEm()),
	}
	gid, ok := f.GlyphIndex('О')
	if !ok {
		t.Skip("test font has no О")
	}
	run, err := text.Shape(f, face.Size, "О")
	if err != nil {
		t.Fatal(err)
	}
	page := newPage(300000, 300000, layout.Op{
		Kind: layout.TextOpKind,
		Text: &layout.TextOp{
			X: 20000, Y: 250000,
			Face:  face,
			Run:   layoutRun(run),
			Color: css.Color{Set: true},
		},
	})
	img, err := Render(page, Options{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if countDark(img) == 0 {
		t.Fatal("the glyph was not drawn")
	}
	// The centre of the О must stay white: it is a ring, not a disc.
	scale := float64(img.Rect.Dx()) / float64(page.Size.Width)
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	unit := float64(face.Size) / float64(f.UnitsPerEm())
	contours, err := f.GlyphPath(gid)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range contours {
		for _, p := range c {
			x := (20000 + float64(p.X)*unit) * scale
			y := (250000 - float64(p.Y)*unit) * scale
			minX = math.Min(minX, x)
			maxX = math.Max(maxX, x)
			minY = math.Min(minY, y)
			maxY = math.Max(maxY, y)
		}
	}
	cx := int((minX + maxX) / 2)
	cy := int((minY + maxY) / 2)
	if cx < 1 || cy < 1 || cx >= img.Rect.Dx()-1 || cy >= img.Rect.Dy()-1 {
		t.Fatalf("glyph bbox is outside the image: %.1f,%.1f..%.1f,%.1f", minX, minY, maxX, maxY)
	}
	if r, _, _, _ := img.At(cx, cy).RGBA(); r < 0x8000 {
		t.Fatal("centre of О is dark, the contour was filled as a solid shape")
	}
	// And a point on the outer contour must be dark.
	if r, _, _, _ := img.At(int(minX)+2, cy).RGBA(); r > 0x8000 {
		t.Fatal("the outline of О was not drawn")
	}
}

func layoutRun(r text.Run) layout.TextRun {
	glyphs := make([]layout.Glyph, len(r.Glyphs))
	for i, g := range r.Glyphs {
		glyphs[i] = layout.Glyph{GID: g.GID, Advance: g.Advance, Rune: g.Rune}
	}
	return layout.TextRun{Glyphs: glyphs, Width: r.Width}
}

func TestDiffDetectsShift(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 20, 20))
	b := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := range a.Pix {
		a.Pix[i] = 255
		b.Pix[i] = 255
	}
	a.Set(10, 10, color.RGBA{R: 0, A: 255})
	b.Set(11, 10, color.RGBA{R: 0, A: 255})
	changed, err := Diff(a, b, 8)
	if err != nil {
		t.Fatal(err)
	}
	if changed == 0 {
		t.Fatal("a one pixel shift was not detected")
	}
}

func TestDiffToleratesAntiAliasing(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 20, 20))
	b := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := range a.Pix {
		a.Pix[i] = 255
		b.Pix[i] = 255
	}
	a.Set(5, 5, color.RGBA{R: 200, G: 200, B: 200, A: 255})
	b.Set(5, 5, color.RGBA{R: 205, G: 205, B: 205, A: 255})
	changed, err := Diff(a, b, 8)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Fatalf("small differences were reported as %d changed pixels", changed)
	}
}

func loadFont(t *testing.T) *font.Font {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	)
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f, err := font.Parse(data)
		if err != nil {
			continue
		}
		return f
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
	return nil
}

func TestImageAlphaCompositesOverThePage(t *testing.T) {
	pngBytes := func(a uint8) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: a})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	page := newPage(100000, 100000, layout.Op{
		Kind:  layout.ImageOpKind,
		Rect:  layout.Rect{X: 0, Y: 0, W: 100000, H: 100000},
		Image: &layout.ImageOp{Image: &layout.ImageData{Data: pngBytes(128), Format: "png", PixelW: 1, PixelH: 1}},
	})
	img, err := Render(page, Options{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(1, 1).RGBA()
	if a != 0xffff {
		t.Fatalf("the page must stay opaque, alpha = %#x", a)
	}
	if r != 0xffff || g < 0x7c00 || g > 0x8200 || b < 0x7c00 || b > 0x8200 {
		t.Fatalf("half-transparent red over white = %#x,%#x,%#x, want ff,~7f,~7f", r, g, b)
	}
}

func TestRenderRejectsOversizedPage(t *testing.T) {
	// 10,000 pt square at 96 dpi is about 13,333 x 13,333 pixels: 177 million,
	// larger than the default budget.
	page := newPage(10_000_000, 10_000_000)
	if _, err := Render(page, Options{DPI: 96}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("oversized page returned %v, want a budget error", err)
	}
	if _, err := Render(newPage(100_000, 100_000), Options{DPI: 96, MaxPixels: 10}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("custom pixel budget returned %v, want a budget error", err)
	}
}

func solidPNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOversizedImageIsClippedToThePage(t *testing.T) {
	// The image rectangle is a million times wider than the page. Only the
	// visible part may allocate a buffer.
	page := newPage(10_000, 10_000, layout.Op{
		Kind:  layout.ImageOpKind,
		Rect:  layout.Rect{X: 0, Y: 0, W: 2_000_000_000, H: 2_000_000_000},
		Image: &layout.ImageOp{Image: &layout.ImageData{Data: solidPNG(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255}), Format: "png", PixelW: 1, PixelH: 1}},
	})
	img, err := Render(page, Options{DPI: 72, MaxPixels: 1000})
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r < 0xfe00 || g < 0xfe00 || b < 0xfe00 {
		t.Fatalf("the visible corner was not painted: %#x,%#x,%#x", r, g, b)
	}
}

func TestRenderHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page := newPage(100000, 100000, layout.Op{Kind: layout.FillOpKind, Rect: layout.Rect{W: 1000, H: 1000}, Color: css.Color{Set: true}})
	if _, err := Render(page, Options{DPI: 72, Budget: budget.New(ctx, 0)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Render returned %v, want context.Canceled", err)
	}
}
