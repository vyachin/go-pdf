package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/font"
)

func loadFont(t *testing.T) *font.Font {
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

func TestSinglePageDocument(t *testing.T) {
	d := New(Options{Title: "Test"})
	d.AddPage(595276, 841890, []Op{
		{Kind: FillOpKind, Rect: Rect{X: 1000, Y: 1000, W: 5000, H: 5000}, Color: RGB{Set: true}},
	})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.7")) {
		t.Fatalf("bad header: %q", data[:8])
	}
	if !bytes.Contains(data, []byte("/Type /Page ")) {
		t.Fatal("no page object")
	}
	if !bytes.Contains(data, []byte("xref")) {
		t.Fatal("no xref table")
	}
}

func TestNoPagesIsAnError(t *testing.T) {
	if _, err := New(Options{}).Bytes(); err == nil {
		t.Fatal("expected an error for a document without pages")
	}
}

func TestDeterministicOutput(t *testing.T) {
	build := func() []byte {
		d := New(Options{Title: "x"})
		d.AddPage(595276, 841890, []Op{
			{Kind: FillOpKind, Rect: Rect{X: 0, Y: 0, W: 1000, H: 1000}, Color: RGB{R: 255, Set: true}},
		})
		data, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if !bytes.Equal(build(), build()) {
		t.Fatal("two runs differ")
	}
}

func TestCreationDateIsStableByDefault(t *testing.T) {
	d := New(Options{})
	d.AddPage(1000, 1000, nil)
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("D:20000101000000Z")) {
		t.Fatal("the default creation date is not the fixed constant")
	}
	withDate := New(Options{CreationDate: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)})
	withDate.AddPage(1000, 1000, nil)
	data, err = withDate.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("D:20260201100000Z")) {
		t.Fatal("the explicit creation date was not used")
	}
}

func TestTextAndToUnicode(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('С')
	if !ok {
		t.Skip("test font has no С")
	}
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{
			X: 10000, Y: 20000,
			Font:   f,
			Size:   12000,
			Glyphs: []Glyph{{GID: gid, Rune: 'С'}},
		},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/ToUnicode")) {
		t.Fatal("no ToUnicode map")
	}
	if !bytes.Contains(data, []byte("/Identity-H")) {
		t.Fatal("text is not encoded as Identity-H")
	}
}

func FuzzWriter(f *testing.F) {
	f.Add(int32(1000), int32(1000), []byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Fuzz(func(t *testing.T, w, h int32, seed []byte) {
		if w <= 0 || h <= 0 || w > 20000 || h > 20000 {
			return
		}
		d := New(Options{})
		var ops []Op
		for i := 0; i+8 <= len(seed); i += 8 {
			ops = append(ops, Op{
				Kind:  FillOpKind,
				Rect:  Rect{X: int32(seed[i]), Y: int32(seed[i+1]), W: int32(seed[i+2]), H: int32(seed[i+3])},
				Color: RGB{R: seed[i+4], G: seed[i+5], B: seed[i+6], Set: true},
			})
		}
		d.AddPage(w, h, ops)
		_, _ = d.Bytes()
	})
}

// pdfObjects splits a PDF into numbered object bodies for structural checks.
func pdfObjects(t *testing.T, data []byte) map[int]string {
	t.Helper()
	out := map[int]string{}
	rest := data
	for {
		idx := bytes.Index(rest, []byte(" obj\n"))
		if idx < 0 {
			return out
		}
		lineStart := bytes.LastIndexByte(rest[:idx], '\n') + 1
		fields := strings.Fields(string(rest[lineStart:idx]))
		rest = rest[idx+len(" obj\n"):]
		end := bytes.Index(rest, []byte("\nendobj"))
		if end < 0 || len(fields) == 0 {
			return out
		}
		id, err := strconv.Atoi(fields[0])
		if err == nil {
			out[id] = string(rest[:end])
		}
		rest = rest[end+len("\nendobj"):]
	}
}

// ref extracts "N 0 R" that follows the given key.
func ref(body, key string) int {
	i := strings.Index(body, key)
	if i < 0 {
		return 0
	}
	rest := body[i+len(key):]
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return 0
	}
	fields[0] = strings.TrimPrefix(fields[0], "[")
	id, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}
	return id
}

// decodeStream inflates a FlateDecode stream object.
func decodeStream(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "stream\n")
	end := strings.Index(body, "\nendstream")
	if start < 0 || end < 0 || end < start {
		return body
	}
	raw := []byte(body[start+len("stream\n") : end])
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		return string(raw)
	}
	return buf.String()
}

func TestFontObjectGraph(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('С')
	if !ok {
		t.Skip("test font has no С")
	}
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 2000, Font: f, Size: 12000, Glyphs: []Glyph{{GID: gid, Rune: 'С'}}},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	objects := pdfObjects(t, data)

	var type0 int
	for id, body := range objects {
		if strings.Contains(body, "/Subtype /Type0") {
			type0 = id
			break
		}
	}
	if type0 == 0 {
		t.Fatal("no Type0 font object")
	}
	cidID := ref(objects[type0], "/DescendantFonts")
	if cidID == 0 {
		t.Fatal("the Type0 font has no /DescendantFonts reference")
	}
	cid := objects[cidID]
	if !strings.Contains(cid, "/Subtype /CIDFontType2") {
		t.Fatalf("/DescendantFonts points at an object without /CIDFontType2: %s", cid)
	}
	descID := ref(cid, "/FontDescriptor")
	if descID == 0 {
		t.Fatal("the CID font has no /FontDescriptor reference")
	}
	desc := objects[descID]
	if !strings.Contains(desc, "/Type /FontDescriptor") {
		t.Fatalf("/FontDescriptor points at: %s", desc)
	}
	fileID := ref(desc, "/FontFile2")
	if fileID == 0 || !strings.Contains(objects[fileID], "stream") {
		t.Fatalf("/FontFile2 does not reference a stream: %q", objects[fileID])
	}
	toUniID := ref(objects[type0], "/ToUnicode")
	if toUniID == 0 || toUniID == cidID || toUniID == descID || toUniID == fileID {
		t.Fatalf("/ToUnicode must be a separate object, got %d", toUniID)
	}
	if !strings.Contains(decodeStream(t, objects[toUniID]), "begincmap") {
		t.Fatal("/ToUnicode does not reference a CMap stream")
	}
}

func testImage(t *testing.T, fill color.RGBA) *Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &Image{Data: buf.Bytes(), Format: "png", PixelW: 4, PixelH: 4}
}

func TestImageResourcesAreDeterministic(t *testing.T) {
	red := testImage(t, color.RGBA{R: 255, A: 255})
	green := testImage(t, color.RGBA{G: 255, A: 255})
	blue := testImage(t, color.RGBA{B: 255, A: 255})
	build := func() []byte {
		d := New(Options{})
		d.AddPage(595276, 841890, []Op{
			{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 2000, H: 2000}, Image: red},
			{Kind: ImageOpKind, Rect: Rect{X: 4000, Y: 1000, W: 2000, H: 2000}, Image: green},
			{Kind: ImageOpKind, Rect: Rect{X: 7000, Y: 1000, W: 2000, H: 2000}, Image: blue},
		})
		data, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	first := build()
	for i := 0; i < 30; i++ {
		if !bytes.Equal(first, build()) {
			t.Fatalf("serialisation %d differs", i+2)
		}
	}
	// The resource dictionary must follow first-use order.
	var pageDict string
	for _, body := range pdfObjects(t, first) {
		if strings.Contains(body, "/Type /Page ") {
			pageDict = body
		}
	}
	first1 := strings.Index(pageDict, "/Im1 ")
	first2 := strings.Index(pageDict, "/Im2 ")
	first3 := strings.Index(pageDict, "/Im3 ")
	if first1 < 0 || first2 < 0 || first3 < 0 || !(first1 < first2 && first2 < first3) {
		t.Fatalf("image resources are out of document order: %d %d %d", first1, first2, first3)
	}
}

func TestKerningAdjustmentSign(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('A')
	if !ok {
		t.Skip("test font has no A")
	}
	second, ok := f.GlyphIndex('V')
	if !ok {
		t.Skip("test font has no V")
	}
	const size = int32(12000)
	w1000 := math.Round(float64(f.Advance(gid)) * 1000 / float64(f.UnitsPerEm()))
	nominal := int32(math.Round(w1000 * float64(size) / 1000))
	tightened := nominal - 500 // layout applied kerning of 500 millipoints

	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{
			X: 1000, Y: 1000, Font: f, Size: size,
			Glyphs: []Glyph{
				{GID: gid, Rune: 'A', Advance: tightened},
				{GID: second, Rune: 'V', Advance: nominal},
			},
		},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	stream := ""
	for _, obj := range pdfObjects(t, data) {
		if s := decodeStream(t, obj); strings.Contains(s, "TJ") {
			stream = s
		}
	}
	if stream == "" {
		t.Fatal("no text run written as a TJ array")
	}
	open := strings.Index(stream, "[")
	closeIdx := strings.Index(stream, "] TJ")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		t.Fatalf("malformed TJ array: %q", stream)
	}
	array := stream[open+1 : closeIdx]
	parts := strings.Fields(array)
	if len(parts) < 3 {
		t.Fatalf("TJ array = %q, want glyph, adjustment, glyph", array)
	}
	adj, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("adjustment %q is not a number", parts[1])
	}
	want := int(math.Round(500.0 * 1000 / float64(size)))
	if adj != want {
		t.Fatalf("adjustment = %d, want %d (positive moves the next glyph left)", adj, want)
	}
}

func TestNoAdjustmentWhenAdvancesAreNominal(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('A')
	if !ok {
		t.Skip("test font has no A")
	}
	const size = int32(12000)
	w1000 := math.Round(float64(f.Advance(gid)) * 1000 / float64(f.UnitsPerEm()))
	nominal := int32(math.Round(w1000 * float64(size) / 1000))
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 1000, Font: f, Size: size, Glyphs: []Glyph{{GID: gid, Rune: 'A', Advance: nominal}}},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range pdfObjects(t, data) {
		if s := decodeStream(t, obj); strings.Contains(s, "TJ") && strings.Contains(s, "] TJ") {
			open := strings.Index(s, "[")
			closeIdx := strings.Index(s, "] TJ")
			if open >= 0 && closeIdx > open {
				if fields := strings.Fields(s[open+1 : closeIdx]); len(fields) != 1 {
					t.Fatalf("a run without kerning must carry no adjustments: %q", s[open:closeIdx+3])
				}
			}
		}
	}
}

func TestPageResourcesListOnlyUsedImages(t *testing.T) {
	first := testImage(t, color.RGBA{R: 255, A: 255})
	second := testImage(t, color.RGBA{G: 255, A: 255})
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 2000, H: 2000}, Image: first}})
	d.AddPage(595276, 841890, []Op{{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 2000, H: 2000}, Image: second}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var pages []string
	for _, body := range pdfObjects(t, data) {
		if strings.Contains(body, "/Type /Page ") {
			pages = append(pages, body)
		}
	}
	if len(pages) != 2 {
		t.Fatalf("got %d page dictionaries, want 2", len(pages))
	}
	withFirst, withSecond := 0, 0
	for _, page := range pages {
		hasFirst := strings.Contains(page, "/Im1 ")
		hasSecond := strings.Contains(page, "/Im2 ")
		if hasFirst && hasSecond {
			t.Fatalf("a page lists both images: %s", page)
		}
		if hasFirst {
			withFirst++
		}
		if hasSecond {
			withSecond++
		}
	}
	if withFirst != 1 || withSecond != 1 {
		t.Fatalf("image resources are not per page: first=%d second=%d", withFirst, withSecond)
	}
}

func TestWidthRoundingMatchesAdjustment(t *testing.T) {
	f := loadFont(t)
	var gid uint16
	found := false
	for g := uint16(0); g < 4000; g++ {
		units := f.Advance(g)
		if units == 0 {
			continue
		}
		exact := float64(units) * 1000 / float64(f.UnitsPerEm())
		if exact-math.Floor(exact) > 0.5 {
			gid, found = g, true
			break
		}
	}
	if !found {
		t.Skip("the test font has no glyph with a fractional width above 0.5")
	}
	exact := float64(f.Advance(gid)) * 1000 / float64(f.UnitsPerEm())
	want := int32(math.Round(exact))
	const size = int32(100000)
	nominal := int32(math.Round(float64(want) * float64(size) / 1000))

	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 1000, Font: f, Size: size, Glyphs: []Glyph{{GID: gid, Advance: nominal}}},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	objects := pdfObjects(t, data)
	// The /W array must carry the rounded width, not a truncated one.
	if !strings.Contains(findFontObjects(objects), fmt.Sprintf("/W [%d [%d]]", gid, want)) {
		t.Fatalf("/W does not round %0.3f to %d", exact, want)
	}
	// An advance equal to the rounded nominal width needs no adjustment.
	for _, body := range objects {
		if s := decodeStream(t, body); strings.Contains(s, "TJ") {
			open := strings.Index(s, "[")
			closeIdx := strings.Index(s, "] TJ")
			if open >= 0 && closeIdx > open {
				if fields := strings.Fields(s[open+1 : closeIdx]); len(fields) != 1 {
					t.Fatalf("a matching nominal width must need no adjustment: %q", s[open:closeIdx+3])
				}
			}
		}
	}
}

func findFontObjects(objects map[int]string) string {
	var b strings.Builder
	for _, body := range objects {
		if strings.Contains(body, "/Subtype /Type0") || strings.Contains(body, "/CIDFontType2") {
			b.WriteString(body)
			b.WriteString("\n")
		}
	}
	return b.String()
}
