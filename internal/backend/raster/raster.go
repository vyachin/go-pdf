// Package raster renders layout pages into images. It is used by tests for
// visual regression and by the CLI debug flag, and it shares the outline data
// of the font package, so no external rasterizer is needed.
package raster

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"sort"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/layout"
)

// Options controls rasterization.
type Options struct {
	// DPI is the output resolution; 96 gives roughly one pixel per CSS px.
	DPI int
	// MaxPixels bounds the area of the output image and of every intermediate
	// scaled image. Zero means the default of 40,000,000 pixels. A page and an
	// image are checked before the buffer is allocated.
	MaxPixels int
}

// defaultMaxPixels matches the image pixel budget of the engine. A page is
// rasterized only on demand, so the default is generous; it exists to reject
// a page geometry that would ask for gigabytes.
const defaultMaxPixels = 40_000_000

type point struct {
	x, y float64
}

// Diff compares two images and returns the number of pixels that differ by
// more than tolerance in any channel. It is used for visual regression tests.
func Diff(a, b *image.RGBA, tolerance int) (int, error) {
	if a.Rect.Dx() != b.Rect.Dx() || a.Rect.Dy() != b.Rect.Dy() {
		return 0, fmt.Errorf("raster: image sizes differ: %v and %v", a.Rect, b.Rect)
	}
	changed := 0
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := int(a.Pix[i+c]) - int(b.Pix[i+c])
			if d < 0 {
				d = -d
			}
			if d > tolerance {
				changed++
				break
			}
		}
	}
	return changed, nil
}

// Render draws a page into an RGBA image. A page whose output would exceed the
// pixel budget is rejected with budget.ErrExceeded before the buffer is
// allocated.
func Render(page *layout.Page, opts Options) (*image.RGBA, error) {
	if opts.DPI <= 0 {
		opts.DPI = 96
	}
	if opts.MaxPixels <= 0 {
		opts.MaxPixels = defaultMaxPixels
	}
	w := int(float64(page.Size.Width) / 1000 * float64(opts.DPI) / 72)
	h := int(float64(page.Size.Height) / 1000 * float64(opts.DPI) / 72)
	if w <= 0 || h <= 0 {
		w, h = 1, 1
	}
	if exceedsArea(w, h, opts.MaxPixels) {
		return nil, fmt.Errorf("%w: a %dx%d page raster exceeds the limit of %d pixels", budget.ErrExceeded, w, h, opts.MaxPixels)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// White page, fully opaque.
	for i := 0; i < len(img.Pix); i++ {
		img.Pix[i] = 255
	}
	scaleX := float64(w) / float64(page.Size.Width)
	scaleY := float64(h) / float64(page.Size.Height)
	for _, op := range page.Ops {
		switch op.Kind {
		case layout.FillOpKind:
			col := toColor(op.Color)
			fillRect(img, op.Rect, scaleX, scaleY, col)
		case layout.TextOpKind:
			drawText(img, op.Text, scaleX, scaleY)
		case layout.ImageOpKind:
			drawImage(img, op, scaleX, scaleY)
		}
	}
	return img, nil
}

// exceedsArea reports whether w*h is larger than limit without overflowing the
// multiplication.
func exceedsArea(w, h, limit int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if limit <= 0 {
		return false
	}
	return w > limit/h
}

func toColor(c css.Color) color.RGBA {
	if !c.Set {
		return color.RGBA{A: 0}
	}
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}
}

func fillRect(img *image.RGBA, rect layout.Rect, sx, sy float64, col color.RGBA) {
	x0 := int(math.Floor(float64(rect.X) * sx))
	y0 := int(math.Floor(float64(rect.Y) * sy))
	x1 := int(math.Ceil(float64(rect.X+rect.W) * sx))
	y1 := int(math.Ceil(float64(rect.Y+rect.H) * sy))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > img.Rect.Dx() {
		x1 = img.Rect.Dx()
	}
	if y1 > img.Rect.Dy() {
		y1 = img.Rect.Dy()
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, col)
		}
	}
}

func drawText(img *image.RGBA, t *layout.TextOp, sx, sy float64) {
	if t == nil || t.Face == nil {
		return
	}
	f := t.Face.Font
	unitToMP := float64(t.Face.Size) / float64(f.UnitsPerEm())
	col := color.RGBA{}
	if t.Color.Set {
		col = color.RGBA{R: t.Color.R, G: t.Color.G, B: t.Color.B, A: 255}
	} else {
		col.A = 255
	}
	x := float64(t.X)
	for _, g := range t.Run.Glyphs {
		contours := f.GlyphPath(g.GID)
		polys := make([][]point, 0, len(contours))
		for _, c := range contours {
			poly := flattenContour(c)
			for i := range poly {
				poly[i] = point{
					x: (x + poly[i].x*unitToMP) * sx,
					y: (float64(t.Y) - poly[i].y*unitToMP) * sy,
				}
			}
			polys = append(polys, poly)
		}
		// All contours of a glyph are rasterized together so that holes
		// (counter-clockwise inner rings) cancel out under the non-zero rule.
		fillPolygons(img, polys, col)
		x += float64(g.Advance)
	}
}

func drawImage(img *image.RGBA, op layout.Op, sx, sy float64) {
	if op.Image == nil || op.Image.Image == nil {
		return
	}
	src := decode(op.Image.Image.Data, op.Image.Image.Format)
	if src == nil {
		return
	}
	b := src.Bounds()
	x0 := int(float64(op.Rect.X) * sx)
	y0 := int(float64(op.Rect.Y) * sy)
	w := int(float64(op.Rect.W) * sx)
	h := int(float64(op.Rect.H) * sy)
	if w <= 0 || h <= 0 {
		return
	}
	// Only the visible part of the image needs a scaled buffer. An image that
	// is wider than the page would otherwise allocate its full CSS size.
	dst := image.Rect(x0, y0, x0+w, y0+h).Intersect(img.Bounds())
	if dst.Empty() {
		return
	}
	// Scale into a straight buffer, then composite with draw.Over so that
	// translucent pixels blend with what is already on the page instead of
	// replacing it.
	scaled := image.NewRGBA(image.Rect(0, 0, dst.Dx(), dst.Dy()))
	for y := 0; y < dst.Dy(); y++ {
		yf := y + dst.Min.Y - y0
		py := b.Min.Y + yf*b.Dy()/maxInt(h, 1)
		if py >= b.Max.Y {
			py = b.Max.Y - 1
		}
		for x := 0; x < dst.Dx(); x++ {
			xf := x + dst.Min.X - x0
			px := b.Min.X + xf*b.Dx()/maxInt(w, 1)
			if px >= b.Max.X {
				px = b.Max.X - 1
			}
			scaled.Set(x, y, src.At(px, py))
		}
	}
	draw.Draw(img, dst, scaled, image.Point{}, draw.Over)
}

func decode(data []byte, format string) image.Image {
	switch format {
	case "png":
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return img
	case "jpeg":
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return img
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// flattenContour converts a TrueType contour into a polygon.
func flattenContour(c font.Contour) []point {
	if len(c) == 0 {
		return nil
	}
	// Insert implied on-curve points between consecutive off-curve points.
	pts := make([]font.Point, 0, len(c)*2)
	for i, p := range c {
		next := c[(i+1)%len(c)]
		pts = append(pts, p)
		if !p.OnCurve && !next.OnCurve {
			pts = append(pts, font.Point{X: (p.X + next.X) / 2, Y: (p.Y + next.Y) / 2, OnCurve: true})
		}
	}
	start := -1
	for i, p := range pts {
		if p.OnCurve {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	n := len(pts)
	out := make([]point, 0, n*4)
	cur := point{float64(pts[start].X), float64(pts[start].Y)}
	out = append(out, cur)
	const steps = 6
	for step, i := 0, (start+1)%n; step < n; {
		p := pts[i]
		if p.OnCurve {
			cur = point{float64(p.X), float64(p.Y)}
			out = append(out, cur)
			i = (i + 1) % n
			step++
			continue
		}
		next := pts[(i+1)%n]
		ctrl := point{float64(p.X), float64(p.Y)}
		end := point{float64(next.X), float64(next.Y)}
		for t := 1; t <= steps; t++ {
			tt := float64(t) / steps
			u := 1 - tt
			out = append(out, point{
				x: u*u*cur.x + 2*u*tt*ctrl.x + tt*tt*end.x,
				y: u*u*cur.y + 2*u*tt*ctrl.y + tt*tt*end.y,
			})
		}
		cur = end
		i = (i + 2) % n
		step += 2
	}
	return out
}

type edge struct {
	x0, y0, x1, y1 float64
	dir            int
}

// fillPolygon rasterizes one closed polygon with non-zero winding.
func fillPolygon(img *image.RGBA, poly []point, col color.RGBA) {
	fillPolygons(img, [][]point{poly}, col)
}

// fillPolygons rasterizes a set of contours as one shape with the non-zero
// winding rule, which is what TrueType outlines expect.
func fillPolygons(img *image.RGBA, polys [][]point, col color.RGBA) {
	var edges []edge
	for _, poly := range polys {
		if len(poly) < 3 {
			continue
		}
		for i := range poly {
			a, b := poly[i], poly[(i+1)%len(poly)]
			if a.y == b.y {
				continue
			}
			dir := 1
			if b.y < a.y {
				dir = -1
				a, b = b, a
			}
			edges = append(edges, edge{x0: a.x, y0: a.y, x1: b.x, y1: b.y, dir: dir})
		}
	}
	if len(edges) == 0 {
		return
	}
	const ss = 4
	w := img.Rect.Dx()
	crossings := make([]struct {
		x   float64
		dir int
	}, 0, len(edges))
	coverage := make([]float32, w)
	for y := 0; y < img.Rect.Dy(); y++ {
		for i := range coverage {
			coverage[i] = 0
		}
		for sub := 0; sub < ss; sub++ {
			sy := float64(y) + (float64(sub)+0.5)/ss
			crossings = crossings[:0]
			for _, e := range edges {
				if sy < e.y0 || sy >= e.y1 {
					continue
				}
				t := (sy - e.y0) / (e.y1 - e.y0)
				crossings = append(crossings, struct {
					x   float64
					dir int
				}{e.x0 + t*(e.x1-e.x0), e.dir})
			}
			if len(crossings) < 2 {
				continue
			}
			sort.Slice(crossings, func(i, j int) bool { return crossings[i].x < crossings[j].x })
			winding := 0
			spanStart := 0.0
			for _, c := range crossings {
				if winding == 0 {
					spanStart = c.x
				}
				winding += c.dir
				if winding == 0 {
					addSpan(coverage, spanStart, c.x, 1.0/ss)
				}
			}
		}
		row := y * img.Stride
		for x := 0; x < w; x++ {
			cv := coverage[x]
			if cv <= 0 {
				continue
			}
			if cv > 1 {
				cv = 1
			}
			i := row + x*4
			blend(img.Pix[i:i+4], col, cv)
		}
	}
}

func addSpan(cov []float32, x0, x1 float64, weight float64) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if x1 <= 0 || x0 >= float64(len(cov)) {
		return
	}
	start := int(math.Floor(x0))
	end := int(math.Ceil(x1))
	if start < 0 {
		start = 0
	}
	if end > len(cov) {
		end = len(cov)
	}
	for x := start; x < end; x++ {
		left := math.Max(x0, float64(x))
		right := math.Min(x1, float64(x+1))
		if right > left {
			cov[x] += float32((right - left) * weight)
		}
	}
}

func blend(dst []byte, src color.RGBA, a float32) {
	if a <= 0 {
		return
	}
	dst[0] = uint8(float32(dst[0])*(1-a) + float32(src.R)*a)
	dst[1] = uint8(float32(dst[1])*(1-a) + float32(src.G)*a)
	dst[2] = uint8(float32(dst[2])*(1-a) + float32(src.B)*a)
	dst[3] = 255
}
