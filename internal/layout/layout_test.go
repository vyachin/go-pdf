package layout

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
	"github.com/vyachin/go-pdf/internal/text"
)

type testFonts struct {
	f     *font.Font
	faces map[string]*FontFace
}

func (tf *testFonts) Face(families []string, weight int, italic bool, size int32) (*FontFace, error) {
	key := ""
	for _, f := range families {
		key += f + ","
	}
	key += string(rune(weight)) + string(rune(size))
	if face, ok := tf.faces[key]; ok {
		return face, nil
	}
	face := &FontFace{
		Font:    tf.f,
		Size:    size,
		Ascent:  text.Scale(int32(tf.f.Ascent()), size, tf.f.UnitsPerEm()),
		Descent: text.Scale(-int32(tf.f.Descent()), size, tf.f.UnitsPerEm()),
		Key:     key,
	}
	tf.faces[key] = face
	return face, nil
}

type testImages struct{ dir string }

func (ti *testImages) Load(src string) (*ImageData, error) {
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(ti.dir, src)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &ImageData{
		Width:  int32(cfg.Width) * 750,
		Height: int32(cfg.Height) * 750,
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "png",
		Data:   data,
	}, nil
}

func loadTestFont(t *testing.T) *font.Font {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		)
	}
	candidates = append(candidates,
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

type harness struct {
	t      *testing.T
	fonts  *testFonts
	images *testImages
	ctx    Context
}

func newHarness(t *testing.T) *harness {
	f := loadTestFont(t)
	h := &harness{
		t:     t,
		fonts: &testFonts{f: f, faces: map[string]*FontFace{}},
		images: &testImages{dir: t.TempDir()},
	}
	h.ctx = Context{
		Fonts:     h.fonts,
		Images:    h.images,
		PageSize:  css.PageSize{Width: 595276, Height: 841890},
		Margin:    [4]int32{56693, 56693, 56693, 56693},
	}
	return h
}

func (h *harness) run(src string) *Document {
	h.t.Helper()
	return h.runWithSize(src, h.ctx.PageSize, h.ctx.Margin)
}

func (h *harness) runWithSize(src string, size css.PageSize, margin [4]int32) *Document {
	h.t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		h.t.Fatal(err)
	}
	diags := profile.Validate(doc)
	if diags.HasErrors() {
		h.t.Fatalf("validation failed: %v", diags.Lines())
	}
	sheet := profile.ExtractStyleSheets(doc)
	cascade := css.NewCascade(css.DefaultStylesheet(), sheet)
	ps := ParsePageStyle(sheet, size, margin, &diags)
	ctx := h.ctx
	ctx.Cascade = cascade
	ctx.PageStyle = ps
	ctx.PageSize = size
	ctx.Margin = margin
	result, layoutDiags := Run(doc, ctx)
	if layoutDiags.HasErrors() {
		h.t.Fatalf("layout failed: %v", layoutDiags.Lines())
	}
	return result
}

// buildTree exposes the box tree for structural assertions.
func (h *harness) buildTree(src string) *Box {
	h.t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		h.t.Fatal(err)
	}
	sheet := profile.ExtractStyleSheets(doc)
	cascade := css.NewCascade(css.DefaultStylesheet(), sheet)
	ctx := h.ctx
	ctx.Cascade = cascade
	root, diags := Build(doc, ctx)
	if diags.HasErrors() {
		h.t.Fatalf("build failed: %v", diags.Lines())
	}
	return root
}

func textOf(doc *Document) string {
	var out []rune
	for _, p := range doc.Pages {
		for _, op := range p.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			for _, g := range op.Text.Run.Glyphs {
				out = append(out, g.Rune)
			}
		}
	}
	return string(out)
}

func textOps(doc *Document) []*TextOp {
	var out []*TextOp
	for _, p := range doc.Pages {
		for _, op := range p.Ops {
			if op.Kind == TextOpKind && op.Text != nil {
				out = append(out, op.Text)
			}
		}
	}
	return out
}

func TestAnonymousBlocks(t *testing.T) {
	h := newHarness(t)
	root := h.buildTree(`<div>простой текст<p>абзац</p></div>`)
	if len(root.Children) != 1 {
		t.Fatalf("body has %d children, want 1", len(root.Children))
	}
	div := root.Children[0]
	if len(div.Children) != 2 {
		t.Fatalf("div has %d children, want an anonymous block and a paragraph", len(div.Children))
	}
	if div.Children[0].Node != nil {
		t.Fatal("the first child should be an anonymous block")
	}
	if div.Children[1].Node == nil || div.Children[1].Node.Tag != "p" {
		t.Fatalf("second child = %v, want p", div.Children[1].Node)
	}
}

func TestDisplayNoneProducesNoBox(t *testing.T) {
	h := newHarness(t)
	root := h.buildTree(`<div><p style="display: none">скрыт</p><p>виден</p></div>`)
	div := root.Children[0]
	if len(div.Children) != 1 {
		t.Fatalf("div has %d children, want 1", len(div.Children))
	}
}

func TestMarginCollapse(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		p { margin: 0 }
		.a { margin: 0 0 10mm 0 }
		.b { margin: 6mm 0 0 0 }
	</style><p class="a">первый</p><p class="b">второй</p>`)
	ops := textOps(doc)
	if len(ops) < 2 {
		t.Fatalf("expected two text runs, got %d", len(ops))
	}
	// 10mm is larger than 6mm, so the collapsed margin is 10mm = 28346 mp.
	if diff := ops[1].Y - ops[0].Y; diff < 42000 || diff > 43600 {
		t.Fatalf("distance between paragraphs = %d, want about 42746 (line + 10mm margin)", diff)
	}
}

func TestWidthIncludesPaddingAndBorder(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		div { width: 100mm; padding: 5mm; border: 1mm solid #000; background-color: #eeeeee }
	</style><div>ок</div>`)
	var width int32
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Rect.H > 1000 {
			width = op.Rect.W
			break
		}
	}
	if width == 0 {
		t.Fatal("no background rectangle was painted")
	}
	// 100mm + 2*5mm + 2*1mm = 112mm.
	if diff := width - 317481; diff > 600 || diff < -600 {
		t.Fatalf("outer width = %d, want about 317481", width)
	}
}

func TestWhitespaceCollapse(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p>первое
		слово   и
		второе</p>`)
	got := textOf(doc)
	if got != "первое слово и второе" {
		t.Fatalf("text = %q", got)
	}
}

func TestListNumberingWithStart(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<ol start="5"><li>один</li><li>два</li></ol>`)
	got := textOf(doc)
	for _, want := range []string{"5.", "6."} {
		if !strings.Contains(got, want) {
			t.Fatalf("text %q does not contain %q", got, want)
		}
	}
}

func TestInlineImageIncreasesLineHeight(t *testing.T) {
	h := newHarness(t)
	img := image.NewRGBA(image.Rect(0, 0, 60, 30))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	path := filepath.Join(h.images.dir, "logo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	doc := h.run(`<p>до <img src="logo.png"> после</p>`)
	var hasImage bool
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == ImageOpKind {
			hasImage = true
			if op.Rect.H != 22500 {
				t.Fatalf("image height = %d, want 22500 (30px at 96dpi)", op.Rect.H)
			}
		}
	}
	if !hasImage {
		t.Fatal("image was not painted")
	}
}

func TestBreakBeforePage(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>.b { break-before: page }</style>
		<p>первая страница</p><p class="b">вторая страница</p>`)
	if len(doc.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(doc.Pages))
	}
}

func TestUnsupportedValueIsAValidationError(t *testing.T) {
	doc, err := html.Parse([]byte(`<style>p { display: flex }</style><p>x</p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	diags := profile.Validate(doc)
	if !diags.HasErrors() {
		t.Fatal("expected a validation error")
	}
}

