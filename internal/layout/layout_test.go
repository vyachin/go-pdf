package layout

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
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
	// The bundled font keeps results identical on every platform and in CI.
	candidates = append(candidates, filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
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
		t:      t,
		fonts:  &testFonts{f: f, faces: map[string]*FontFace{}},
		images: &testImages{dir: t.TempDir()},
	}
	h.ctx = Context{
		Fonts:    h.fonts,
		Images:   h.images,
		PageSize: css.PageSize{Width: 595276, Height: 841890},
		Margin:   [4]int32{56693, 56693, 56693, 56693},
	}
	return h
}

func (h *harness) run(src string) *Document {
	h.t.Helper()
	return h.runWithSize(src, h.ctx.PageSize, h.ctx.Margin)
}

// runRaw returns layout diagnostics instead of failing the test, for cases
// that are expected to report errors.
func (h *harness) runRaw(src string, size css.PageSize, margin [4]int32) (*Document, diag.List) {
	h.t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		h.t.Fatal(err)
	}
	diags := profile.Validate(doc, profile.Options{})
	sheets, sheetDiags := profile.CollectStylesheets(doc, profile.Options{})
	diags = append(diags, sheetDiags...)
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	ps := ParsePageStyle(sheets, size, margin)
	ctx := h.ctx
	ctx.Cascade = cascade
	ctx.PageStyle = ps
	ctx.PageSize = size
	ctx.Margin = margin
	return Run(doc, ctx)
}

func (h *harness) runWithSize(src string, size css.PageSize, margin [4]int32) *Document {
	h.t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		h.t.Fatal(err)
	}
	diags := profile.Validate(doc, profile.Options{})
	if diags.HasErrors() {
		h.t.Fatalf("validation failed: %v", diags.Lines())
	}
	sheets, _ := profile.CollectStylesheets(doc, profile.Options{})
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	ps := ParsePageStyle(sheets, size, margin)
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
	sheets, _ := profile.CollectStylesheets(doc, profile.Options{})
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
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
	diags := profile.Validate(doc, profile.Options{})
	if !diags.HasErrors() {
		t.Fatal("expected a validation error")
	}
}

func TestTableGridLimit(t *testing.T) {
	h := newHarness(t)
	h.ctx.MaxTableCells = 3
	src := `<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>`
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("table grid limit was not enforced: %v", diags.Lines())
	}
}

func TestTableGridLimitCountsSpans(t *testing.T) {
	h := newHarness(t)
	h.ctx.MaxTableCells = 20
	// Two cells per row, but each spans ten columns: the grid, not the cell
	// count, is what layout allocates.
	src := `<table><tr><td colspan="10">A</td><td colspan="10">B</td></tr>` +
		`<tr><td>C</td><td>D</td></tr></table>`
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("span multiplication was not counted: %v", diags.Lines())
	}
}

func TestLayoutStopsWhenBudgetExceeded(t *testing.T) {
	h := newHarness(t)
	h.ctx.Budget = budget.New(context.Background(), 2)
	src := strings.Repeat("<p>Строка для проверки бюджета раскладки.</p>", 20)
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("layout budget was not enforced: %v", diags.Lines())
	}
}

func TestLayoutStopsWhenCancelled(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.ctx.Budget = budget.New(ctx, 0)
	src := strings.Repeat("<p>Строка для проверки отмены раскладки.</p>", 20)
	doc, _ := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if len(doc.Pages) != 0 {
		t.Fatalf("a cancelled layout produced %d pages", len(doc.Pages))
	}
}

func TestPageLimitStopsForcedBreaks(t *testing.T) {
	h := newHarness(t)
	h.ctx.MaxPages = 1
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(`<p style="break-after:page">A</p>`)
	}
	doc, diags := h.runRaw(b.String(), h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("page limit was not enforced: %v", diags.Lines())
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("layout created %d pages, want at most 1", len(doc.Pages))
	}
}

func TestPageLimitStopsBeforeLayoutEnds(t *testing.T) {
	h := newHarness(t)
	h.ctx.MaxPages = 2
	src := strings.Repeat("<p>Строка документа для проверки лимита страниц.</p>", 400)
	doc, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("page limit was not enforced: %v", diags.Lines())
	}
	if len(doc.Pages) > 2 {
		t.Fatalf("layout created %d pages, want at most 2", len(doc.Pages))
	}
}

func TestHiddenRootDrawsNothing(t *testing.T) {
	cases := map[string]string{
		"body": `<body style="display:none"><h1 id="t">PRIVATE</h1><p>PRIVATE</p></body>`,
		"html": `<style>html{display:none}</style><h1 id="t">PRIVATE</h1><p>PRIVATE</p>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			doc := h.run(src)
			if text := textOf(doc); strings.Contains(text, "PRIVATE") {
				t.Fatalf("hidden root still drew %q", text)
			}
			if len(doc.Outline) != 0 {
				t.Fatalf("hidden root still produced outline entries: %v", doc.Outline)
			}
			if _, ok := doc.Anchors["t"]; ok {
				t.Fatal("hidden root still registered an anchor")
			}
		})
	}
}

func TestIgnoredServiceNodesNeverPrint(t *testing.T) {
	h := newHarness(t)
	src := `<style>script{display:block} style{display:block}</style>` +
		`<script>SECRET</script><style>p{color:red}</style><p>VISIBLE</p>`
	doc := h.run(src)
	text := textOf(doc)
	if strings.Contains(text, "SECRET") || strings.Contains(text, "color:red") {
		t.Fatalf("an ignored service node was printed: %q", text)
	}
	if !strings.Contains(text, "VISIBLE") {
		t.Fatalf("visible text is missing: %q", text)
	}
}

func TestBlockInsideCellKeepsGeometryAndDecorations(t *testing.T) {
	h := newHarness(t)
	block := `<div style="width:30pt;margin:0 0 0 10pt;border:1pt solid red;background-color:blue">A</div>`
	outside := h.run(`<p style="margin:0">Z</p>` + block)
	inside := h.run(`<table><tr><td><p style="margin:0">Z</p>` + block + `</td></tr></table>`)

	type info struct {
		bgW      int32
		leftEdge int32 // background x relative to the cell/body content edge
		textRel  int32 // text x relative to the background
		borders  int
	}
	inspect := func(doc *Document) info {
		var out info
		var bg *Op
		var zX, aX int32
		for i := range doc.Pages[0].Ops {
			op := &doc.Pages[0].Ops[i]
			switch op.Kind {
			case FillOpKind:
				switch op.Color {
				case css.Color{R: 0, G: 0, B: 255, Set: true}:
					bg = op
				case css.Color{R: 255, Set: true}:
					out.borders++
				}
			case TextOpKind:
				if op.Text == nil || len(op.Text.Run.Glyphs) == 0 {
					continue
				}
				switch op.Text.Run.Glyphs[0].Rune {
				case 'Z':
					zX = op.Text.X
				case 'A':
					aX = op.Text.X
				}
			}
		}
		if bg == nil {
			t.Fatal("the block background is missing")
		}
		out.bgW = bg.Rect.W
		out.leftEdge = bg.Rect.X - zX
		out.textRel = aX - bg.Rect.X
		return out
	}

	want := info{
		bgW:      32000, // 30 pt content + two 1 pt borders
		leftEdge: 10000, // margin-left: 10pt
		textRel:  1000,  // border-left: 1pt
		borders:  4,
	}
	if got := inspect(outside); got != want {
		t.Fatalf("block outside a cell = %+v, want %+v", got, want)
	}
	if got := inspect(inside); got != want {
		t.Fatalf("the same block inside a cell = %+v, want %+v", got, want)
	}
}

func TestBlockImageInsideCellReservesHeight(t *testing.T) {
	h := newHarness(t)
	img := image.NewRGBA(image.Rect(0, 0, 60, 30))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, err := os.Create(filepath.Join(h.images.dir, "logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	doc := h.run(`<table><tr><td><img src="logo.png" style="width:50pt;height:30pt"><p style="margin:0">AFTER</p></td></tr></table>`)
	var imageRect Rect
	var afterY int32 = -1
	for _, op := range doc.Pages[0].Ops {
		switch op.Kind {
		case ImageOpKind:
			imageRect = op.Rect
		case TextOpKind:
			if op.Text != nil && len(op.Text.Run.Glyphs) > 0 && op.Text.Run.Glyphs[0].Rune == 'A' {
				afterY = op.Text.Y
			}
		}
	}
	if imageRect.W != 50000 || imageRect.H != 30000 {
		t.Fatalf("image rect = %+v, want 50pt x 30pt", imageRect)
	}
	if afterY < imageRect.Y+imageRect.H {
		t.Fatalf("AFTER baseline %d overlaps the image that ends at %d", afterY, imageRect.Y+imageRect.H)
	}
}

func TestListMarkersInsideCells(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr><td><ol><li>FIRST</li><li>SECOND</li></ol></td></tr></table>`)
	text := textOf(doc)
	if !strings.Contains(text, "1.") || !strings.Contains(text, "2.") {
		t.Fatalf("list markers are missing inside a cell: %q", text)
	}
	if !strings.Contains(text, "FIRST") || !strings.Contains(text, "SECOND") {
		t.Fatalf("list text is missing: %q", text)
	}
	if i, j := strings.Index(text, "1."), strings.Index(text, "FIRST"); i < 0 || j < 0 || i > j {
		t.Fatalf("the marker is not before its item: %q", text)
	}
	// A bullet list keeps its bullet.
	bullets := textOf(h.run(`<table><tr><td><ul><li>ITEM</li></ul></td></tr></table>`))
	if !strings.Contains(bullets, "•") {
		t.Fatalf("bullet marker is missing inside a cell: %q", bullets)
	}
}

func TestAnchorsAndBookmarksInsideCells(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr><td><h2 id="target">TITLE</h2></td></tr></table>`)
	anchor, ok := doc.Anchors["target"]
	if !ok {
		t.Fatal("an id inside a table cell was not registered")
	}
	if anchor.Page != 1 {
		t.Fatalf("anchor page = %d, want 1", anchor.Page)
	}
	if len(doc.Outline) != 1 || doc.Outline[0].Title != "TITLE" {
		t.Fatalf("outline = %+v, want one TITLE entry", doc.Outline)
	}
	// An id on a nested block inside the cell works too.
	doc = h.run(`<table><tr><td><div><p id="deep">DEEP</p></div></td></tr></table>`)
	if _, ok := doc.Anchors["deep"]; !ok {
		t.Fatal("a nested id inside a table cell was not registered")
	}
}

func TestRepeatedHeaderDoesNotDuplicateBookmarks(t *testing.T) {
	h := newHarness(t)
	var rows strings.Builder
	for i := 0; i < 80; i++ {
		rows.WriteString("<tr><td>Row text for pagination.</td></tr>")
	}
	src := `<table><thead><tr><th><h2 id="head">HEAD</h2></th></tr></thead><tbody>` + rows.String() + `</tbody></table>`
	doc := h.run(src)
	if len(doc.Outline) != 1 {
		t.Fatalf("outline has %d entries, want one", len(doc.Outline))
	}
	if len(doc.Pages) < 2 {
		t.Fatalf("the table did not paginate: %d pages", len(doc.Pages))
	}
}

func TestRowStyleHeightAndBackground(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr style="background-color:red;height:60pt"><td>CELL</td></tr></table><p style="margin:0">AFTER</p>`)
	var bg *Op
	var bgIndex, afterIndex = -1, -1
	var afterY int32
	for i := range doc.Pages[0].Ops {
		op := &doc.Pages[0].Ops[i]
		if op.Kind == FillOpKind && op.Color == (css.Color{R: 255, Set: true}) && bgIndex < 0 {
			bg, bgIndex = op, i
		}
		if op.Kind == TextOpKind && op.Text != nil && len(op.Text.Run.Glyphs) > 0 &&
			op.Text.Run.Glyphs[0].Rune == 'A' && afterIndex < 0 && i > bgIndex {
			afterY, afterIndex = op.Text.Y, i
		}
	}
	if bg == nil {
		t.Fatal("the row background was not painted")
	}
	if bg.Rect.H < 60000 {
		t.Fatalf("row background height = %d, want at least 60pt", bg.Rect.H)
	}
	if bgIndex > afterIndex {
		t.Fatal("the row background must be painted before the cell content")
	}
	// The following block starts after the 60 pt row.
	if afterY < bg.Rect.Y+60000 {
		t.Fatalf("AFTER baseline %d overlaps the row that ends at %d", afterY, bg.Rect.Y+60000)
	}
}

func TestRowspanIsClampedToRemainingRows(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr><td rowspan="2">X<br>Y<br>Z</td><td>Q</td></tr></table><p style="margin:0">AFTER</p>`)
	var cY, afterY int32 = -1, -1
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil || len(op.Text.Run.Glyphs) == 0 {
			continue
		}
		switch op.Text.Run.Glyphs[0].Rune {
		case 'Z':
			cY = op.Text.Y
		case 'A':
			afterY = op.Text.Y
		}
	}
	if cY < 0 || afterY < 0 {
		t.Fatalf("missing baselines: C=%d AFTER=%d", cY, afterY)
	}
	if afterY <= cY {
		t.Fatalf("AFTER baseline %d does not follow the cell content that ends at %d", afterY, cY)
	}
}
