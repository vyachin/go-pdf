package layout

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

func TestCoverRect(t *testing.T) {
	a4 := Rect{W: 595276, H: 841890} // A4 with no margins, in millipoints
	cases := []struct {
		name string
		img  *ImageData
		rect Rect
		want Rect
	}{
		{
			// A 2:3 photograph on A4 is wider than the page ratio, so the
			// width decides the scale and the overflow goes to both edges.
			name: "2:3 image on A4",
			img:  &ImageData{Width: 3000, Height: 4500},
			rect: a4,
			want: Rect{X: 0, Y: -25512, W: 595276, H: 892914},
		},
		{
			name: "image wider than the box",
			img:  &ImageData{Width: 4000, Height: 1000},
			rect: Rect{W: 1000, H: 1000},
			want: Rect{X: -1500, Y: 0, W: 4000, H: 1000},
		},
		{
			name: "same ratio fills exactly",
			img:  &ImageData{Width: 600, Height: 300},
			rect: Rect{W: 1000, H: 500},
			want: Rect{X: 0, Y: 0, W: 1000, H: 500},
		},
		{
			name: "a degenerate image paints nothing",
			img:  &ImageData{},
			rect: a4,
			want: Rect{},
		},
		{
			name: "a degenerate box paints nothing",
			img:  &ImageData{Width: 10, Height: 10},
			rect: Rect{},
			want: Rect{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coverRect(tc.img, tc.rect); got != tc.want {
				t.Fatalf("coverRect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// writeCoverPNG creates a solid PNG of the given pixel size in the harness
// image directory.
func writeCoverPNG(t *testing.T, dir, name string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundImageIsPaintedBehindTextAndClipped(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "cover.png", 60, 30)
	doc := h.runWithSize(`<style>
		div { width: 100pt; height: 50pt; background-image: url(cover.png); background-size: cover }
	</style><div>Hello world</div>`, css.PageSize{Width: 595276, Height: 841890}, [4]int32{})

	var imgIndex, textIndex = -1, -1
	var imgOp Op
	for i, op := range doc.Pages[0].Ops {
		switch op.Kind {
		case ImageOpKind:
			if imgIndex < 0 {
				imgIndex, imgOp = i, op
			}
		case TextOpKind:
			if textIndex < 0 {
				textIndex = i
			}
		}
	}
	if imgIndex < 0 || textIndex < 0 {
		t.Fatalf("image index %d, text index %d: both are required", imgIndex, textIndex)
	}
	if imgIndex > textIndex {
		t.Fatalf("the background is painted after the text: image %d, text %d", imgIndex, textIndex)
	}
	want := Rect{X: 0, Y: 0, W: 100000, H: 50000}
	if imgOp.Rect != want {
		t.Fatalf("covered rect = %+v, want %+v", imgOp.Rect, want)
	}
	if imgOp.Image == nil || imgOp.Image.Clip == nil {
		t.Fatal("the covered image has no clip")
	}
	if *imgOp.Image.Clip != want {
		t.Fatalf("clip = %+v, want the box %+v", *imgOp.Image.Clip, want)
	}
}

func TestBackgroundImageCropsEquallyOnBothSides(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "wide.png", 60, 30)
	doc := h.runWithSize(`<style>
		div { width: 50pt; height: 100pt; background-image: url(wide.png); background-size: cover }
	</style><div>x</div>`, css.PageSize{Width: 595276, Height: 841890}, [4]int32{})

	var imgOp *Op
	for i := range doc.Pages[0].Ops {
		if doc.Pages[0].Ops[i].Kind == ImageOpKind {
			imgOp = &doc.Pages[0].Ops[i]
			break
		}
	}
	if imgOp == nil {
		t.Fatal("no background image was painted")
	}
	// A 2:1 image in a 1:2 box scales to 200pt wide; the 150pt overflow is
	// split into 75pt on each side.
	want := Rect{X: -75000, Y: 0, W: 200000, H: 100000}
	if imgOp.Rect != want {
		t.Fatalf("covered rect = %+v, want %+v", imgOp.Rect, want)
	}
	if imgOp.Image == nil || imgOp.Image.Clip == nil || *imgOp.Image.Clip != (Rect{W: 50000, H: 100000}) {
		t.Fatalf("clip = %+v, want the 50x100pt box", imgOp.Image)
	}
}

func TestBackgroundImageMissingFileIsADiagnostic(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(`<div style="background-image: url(missing.png)">x</div>`,
		css.PageSize{Width: 595276, Height: 841890}, [4]int32{})
	if !diags.HasErrors() {
		t.Fatalf("a missing background image was accepted: %v", diags.Lines())
	}
	if !strings.Contains(strings.Join(diags.Lines(), "\n"), "missing.png") {
		t.Fatalf("the diagnostic does not name the file: %v", diags.Lines())
	}
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == ImageOpKind {
			t.Fatal("a missing background image was still painted")
		}
	}
}
