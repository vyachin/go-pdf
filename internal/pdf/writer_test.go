package pdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/vyachin/go-pdf/internal/testfont"
	"go/build"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

func loadFont(t *testing.T) *font.Font {
	t.Helper()
	return testfont.Load(t)
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

func TestUTF16Hex(t *testing.T) {
	cases := []struct {
		r    rune
		want string
	}{
		{'A', "0041"},
		{'Ж', "0416"},
		// The pair a non-BMP character keeps in UTF-16.
		{'\U0001F600', "D83DDE00"},
		{'\U0010FFFF', "DBFFDFFF"},
	}
	for _, tc := range cases {
		if got := utf16Hex(tc.r); got != tc.want {
			t.Fatalf("utf16Hex(%U) = %s, want %s", tc.r, got, tc.want)
		}
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
	defer func() { _ = zr.Close() }()
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
	if first1 < 0 || first2 < 0 || first3 < 0 || first1 >= first2 || first2 >= first3 {
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
	if !strings.Contains(findFontObjects(objects), fmt.Sprintf("/W [0 [%d]]", want)) {
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

func TestPNGAlphaUsesASoftMask(t *testing.T) {
	pixel := func(a uint8) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: a})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	render := func(data []byte) (rgb, mask []byte) {
		obj, err := imageObject(nil, &Image{Data: data, Format: "png", PixelW: 1, PixelH: 1}, func(m []byte) int {
			mask = append([]byte(nil), m...)
			return 7
		})
		if err != nil {
			t.Fatal(err)
		}
		body := decodeStream(t, string(obj))
		return []byte(body), mask
	}
	rgb, mask := render(pixel(255))
	if mask != nil {
		t.Fatalf("an opaque image must not carry a soft mask: %v", mask)
	}
	if !bytes.Equal(rgb, []byte{255, 0, 0}) {
		t.Fatalf("opaque red = %v, want 255,0,0", rgb)
	}
	// A translucent pixel keeps its unpremultiplied colour and gets alpha in
	// the soft mask, so a viewer composes it over the real page background.
	rgb, mask = render(pixel(128))
	if !bytes.Equal(rgb, []byte{255, 0, 0}) {
		t.Fatalf("translucent red stored as %v, want unpremultiplied 255,0,0", rgb)
	}
	// /SMask must reference a complete image XObject: a bare pixel stream makes
	// strict validators reject the file even though relaxed readers cope.
	maskText := string(mask)
	for _, want := range []string{
		"/Type /XObject", "/Subtype /Image", "/Width 1", "/Height 1",
		"/ColorSpace /DeviceGray", "/BitsPerComponent 8", "/Filter /FlateDecode",
	} {
		if !strings.Contains(maskText, want) {
			t.Fatalf("soft mask %q does not contain %q", maskText, want)
		}
	}
	if got := []byte(decodeStream(t, maskText)); len(got) != 1 || got[0] != 128 {
		t.Fatalf("soft mask pixels = %v, want one byte of 128", got)
	}
}

func TestCMYKJPEGIsNotEmbeddedAsRGB(t *testing.T) {
	fixture := filepath.Join(build.Default.GOROOT, "src", "image", "testdata", "video-001.cmyk.jpeg")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("no CMYK fixture in this Go distribution: %v", err)
	}
	_, err = imageObject(nil, &Image{Data: data, Format: "jpeg", PixelW: 1, PixelH: 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "CMYK") {
		t.Fatalf("a four-component JPEG must not be embedded as RGB, got %v", err)
	}
}

// TestJPEGColorSpaceFromTheStandardLibrary checks that the writer reads the
// frame header with image/jpeg: one component is DeviceGray, three is
// DeviceRGB, and the dimensions are the ones the standard library reports.
func TestJPEGColorSpaceFromTheStandardLibrary(t *testing.T) {
	cases := []struct {
		name       string
		fixture    string
		colorSpace string
	}{
		{"grayscale", "video-005.gray.jpeg", "/DeviceGray"},
		{"ycbcr", "video-001.jpeg", "/DeviceRGB"},
		{"rgb", "video-001.rgb.jpeg", "/DeviceRGB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(build.Default.GOROOT, "src", "image", "testdata", tc.fixture)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("no %s fixture in this Go distribution: %v", tc.fixture, err)
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("the fixture does not decode: %v", err)
			}
			obj, err := imageObject(nil, &Image{Data: data, Format: "jpeg"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			text := string(obj)
			for _, want := range []string{
				"/ColorSpace " + tc.colorSpace,
				fmt.Sprintf("/Width %d", cfg.Width),
				fmt.Sprintf("/Height %d", cfg.Height),
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("image object does not contain %q:\n%s", want, text)
				}
			}
		})
	}
}

// TestUnsupportedJPEGIsRejectedWithoutAnImageObject covers a JPEG the standard
// library cannot decode: a 12-bit file is not embedded silently.
func TestUnsupportedJPEGIsRejectedWithoutAnImageObject(t *testing.T) {
	path := filepath.Join(build.Default.GOROOT, "src", "image", "testdata", "video-005.gray.jpeg")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no grayscale fixture in this Go distribution: %v", err)
	}
	// The SOF0 marker holds the sample precision one byte after its length.
	i := bytes.Index(data, []byte{0xFF, 0xC0})
	if i < 0 || i+4 >= len(data) {
		t.Skip("the fixture has no SOF0 marker")
	}
	unsupported := append([]byte(nil), data...)
	unsupported[i+4] = 12
	obj, err := imageObject(nil, &Image{Data: unsupported, Format: "jpeg"}, nil)
	if err == nil {
		t.Fatal("a 12-bit JPEG must be rejected")
	}
	if obj != nil {
		t.Fatalf("a rejected JPEG must not produce an image object: %q", obj)
	}
	if !strings.Contains(err.Error(), "jpeg") {
		t.Fatalf("the error does not name the format: %v", err)
	}
}

func TestSharedGlyphKeepsBothRunes(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('A')
	if !ok {
		t.Skip("test font has no A")
	}
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 2000, Font: f, Size: 12000, Glyphs: []Glyph{
			{GID: gid, Rune: 'A'},
			{GID: gid, Rune: 'Б'},
		}},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	objects := pdfObjects(t, data)
	var cidFont string
	for _, body := range objects {
		if strings.Contains(body, "/CIDFontType2") {
			cidFont = body
			break
		}
	}
	if cidFont == "" {
		t.Fatal("no CIDFontType2 object")
	}
	cidMapID := ref(cidFont, "/CIDToGIDMap")
	cidMap := decodeStream(t, objects[cidMapID])
	if len(cidMap) != 4 || cidMap[0] != byte(gid>>8) || cidMap[1] != byte(gid) ||
		cidMap[2] != byte(gid>>8) || cidMap[3] != byte(gid) {
		t.Fatalf("CIDToGIDMap = % x, want two entries of gid %d", []byte(cidMap), gid)
	}
	var toUni string
	for _, body := range objects {
		if decoded := decodeStream(t, body); strings.Contains(decoded, "begincmap") {
			toUni = decoded
			break
		}
	}
	if !strings.Contains(toUni, "<0000> <0041>") || !strings.Contains(toUni, "<0001> <0411>") {
		t.Fatalf("ToUnicode does not keep both runes:\n%s", toUni)
	}
	// The content stream addresses the two CIDs, not one glyph code.
	var content string
	for _, body := range objects {
		if decoded := decodeStream(t, body); strings.Contains(decoded, "TJ") {
			content += decoded
		}
	}
	// Contiguous glyphs share one string: the two CIDs are the two 2-byte
	// codes of the same hexadecimal string.
	if !strings.Contains(content, "<00000001>") {
		t.Fatalf("content does not address both CIDs: %q", content)
	}
}

func TestBytesHonoursCancellationAfterStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	guard := budget.New(ctx, 0)
	d := New(Options{Budget: guard})
	ops := []Op{{Kind: FillOpKind, Rect: Rect{W: 1000, H: 1000}, Color: RGB{Set: true}}}
	for i := 0; i < 1000; i++ {
		d.AddPage(595276, 841890, ops)
	}
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	if _, err := d.Bytes(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Bytes returned %v, want context.Canceled", err)
	}
}

func TestOutlineCountsAllVisibleDescendants(t *testing.T) {
	entries := []OutlineEntry{
		{Title: "A", Page: 1, Level: 1},
		{Title: "B", Page: 1, Level: 2},
		{Title: "C", Page: 1, Level: 3},
		{Title: "D", Page: 1, Level: 1},
	}
	w := &objectWriter{}
	root := w.add()
	writeOutline(w, entries, root, []*page{{height: 800000}}, []int{99}, root)

	body := func(id int) string { return string(w.bodies[id-1]) }
	if got := body(root); !strings.Contains(got, "/Count 4") {
		t.Fatalf("root /Count is wrong: %s", got)
	}
	// Entry ids follow the root id in insertion order.
	if got := body(root + 1); !strings.Contains(got, "/Count 2") {
		t.Fatalf("A /Count is wrong: %s", got)
	}
	if got := body(root + 2); !strings.Contains(got, "/Count 1") {
		t.Fatalf("B /Count is wrong: %s", got)
	}
	for _, id := range []int{root + 3, root + 4} {
		if got := body(id); strings.Contains(got, "/Count") {
			t.Fatalf("a leaf must not have /Count: %s", got)
		}
	}
}

func TestImageClipIsWrittenBeforeTheDraw(t *testing.T) {
	img := testImage(t, color.RGBA{R: 255, A: 255})
	clip := Rect{X: 1000, Y: 2000, W: 3000, H: 4000}
	content := func(clip *Rect) string {
		d := New(Options{})
		d.AddPage(595276, 841890, []Op{{
			Kind:  ImageOpKind,
			Rect:  Rect{X: 500, Y: 1500, W: 4000, H: 5000},
			Image: img,
			Clip:  clip,
		}})
		data, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		for _, body := range pdfObjects(t, data) {
			if s := decodeStream(t, body); strings.Contains(s, "/Im1 Do") {
				return s
			}
		}
		t.Fatal("no content stream draws the image")
		return ""
	}
	clipped := content(&clip)
	// The rectangle is written in PDF space, where y counts from the bottom:
	// 841.89 - 2 - 4 = 835.89.
	if !strings.Contains(clipped, "q 1 835.89 3 4 re W n") {
		t.Fatalf("the clip rectangle is missing: %q", clipped)
	}
	if strings.Index(clipped, "re W n") > strings.Index(clipped, "Do") {
		t.Fatalf("the clip is applied after the draw: %q", clipped)
	}
	plain := content(nil)
	if strings.Contains(plain, "W n") {
		t.Fatalf("an image without a clip was clipped anyway: %q", plain)
	}
}

// TestNilFontReturnsError guards the writer against a nil font pointer.
func TestNilFontReturnsError(t *testing.T) {
	d := New(Options{})
	d.AddPage(1000, 1000, []Op{{Kind: TextOpKind, Text: &Text{
		X: 0, Y: 0, Size: 12000, Font: nil,
		Glyphs: []Glyph{{GID: 1, Rune: 'A', Advance: 500}},
	}}})
	if _, err := d.Bytes(); err == nil {
		t.Fatal("a nil font was accepted")
	}
}

// TestTooManyCIDsIsAResourceLimit guards the uint16 CID: more than 65,535
// distinct glyph-rune pairs must fail instead of wrapping.
func TestTooManyCIDsIsAResourceLimit(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs := make([]Glyph, 65536)
	for i := range glyphs {
		glyphs[i] = Glyph{GID: 1, Rune: rune(i), Advance: 500}
	}
	d := New(Options{})
	d.AddPage(1000, 1000, []Op{{Kind: TextOpKind, Text: &Text{
		X: 0, Y: 0, Size: 12000, Font: f, Glyphs: glyphs,
	}}})
	_, err = d.Bytes()
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("Bytes returned %v, want a resource limit", err)
	}
}

// TestZeroOutlineLevelIsASibling guards outline normalisation: level 0 is
// level 1, so B is a sibling of A, not its child.
func TestZeroOutlineLevelIsASibling(t *testing.T) {
	w := &objectWriter{}
	rootID := w.add()
	pageID := w.add()
	pages := []*page{{width: 1000, height: 1000}}
	pageIDs := []int{pageID}
	writeOutline(w, []OutlineEntry{
		{Title: "A", Page: 1, Level: 0},
		{Title: "B", Page: 1, Level: 1},
	}, rootID, pages, pageIDs, rootID)
	// add order: root, page, A, B.
	if len(w.bodies) < 4 {
		t.Fatalf("got %d objects", len(w.bodies))
	}
	bBody := string(w.bodies[3])
	if !strings.Contains(bBody, "/Parent 1 0 R") {
		t.Fatalf("B is not a root child: %s", bBody)
	}
	if strings.Contains(bBody, "/Parent 3 0 R") {
		t.Fatalf("B was nested under A: %s", bBody)
	}
}

// italicTestFont loads the test face with a non-zero italic angle in its post
// table. The bundled faces are upright and a test must not depend on a system
// italic font, so the angle field of a real post table is patched in place.
func italicTestFont(t *testing.T, angle float64) *font.Font {
	t.Helper()
	data, err := os.ReadFile(testfont.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	patched := false
	for i := 0; i < numTables; i++ {
		record := data[12+i*16:]
		if string(record[0:4]) != "post" {
			continue
		}
		offset := int(binary.BigEndian.Uint32(record[8:12]))
		if offset+8 > len(data) {
			t.Fatal("the post table is out of bounds")
		}
		if binary.BigEndian.Uint32(data[offset:offset+4]) == 0x00030000 {
			t.Skip("the test font's post table declares no italic angle")
		}
		binary.BigEndian.PutUint32(data[offset+4:offset+8], uint32(int32(angle*65536)))
		patched = true
		break
	}
	if !patched {
		t.Skip("the test font has no post table")
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// fontDescriptorBody returns the /FontDescriptor dictionary of a one-glyph
// document.
func fontDescriptorBody(t *testing.T, face *font.Font) string {
	t.Helper()
	gid, ok := face.GlyphIndex('A')
	if !ok {
		t.Skip("the test font has no A")
	}
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 2000, Font: face, Size: 12000, Glyphs: []Glyph{{GID: gid, Rune: 'A'}}},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range pdfObjects(t, data) {
		if strings.Contains(body, "/Type /FontDescriptor") {
			return body
		}
	}
	t.Fatal("no font descriptor")
	return ""
}

// TestFontDescriptorFlagsMatchItalicAngle guards RV5-34: every descriptor said
// /Flags 4, even for an italic face. Bit 7 marks italics and agrees with the
// /ItalicAngle in the same dictionary.
func TestFontDescriptorFlagsMatchItalicAngle(t *testing.T) {
	cases := []struct {
		name  string
		face  *font.Font
		flags string
		angle string
	}{
		{"upright", loadFont(t), "/Flags 4 ", "/ItalicAngle 0 "},
		{"italic", italicTestFont(t, -12), "/Flags 68 ", "/ItalicAngle -12 "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := fontDescriptorBody(t, tc.face)
			if !strings.Contains(desc, tc.flags) {
				t.Fatalf("descriptor flags are not %s:\n%s", strings.TrimSpace(tc.flags), desc)
			}
			if !strings.Contains(desc, tc.angle) {
				t.Fatalf("descriptor angle is not %s:\n%s", strings.TrimSpace(tc.angle), desc)
			}
		})
	}
}

// textDocument builds a one-page document with the given glyph run.
func textDocument(t *testing.T, face *font.Font, glyphs ...Glyph) []byte {
	t.Helper()
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{{
		Kind: TextOpKind,
		Text: &Text{X: 1000, Y: 2000, Font: face, Size: 12000, Glyphs: glyphs},
	}})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// toUnicodeOf returns the decoded ToUnicode CMap of a document.
func toUnicodeOf(t *testing.T, data []byte) string {
	t.Helper()
	for _, body := range pdfObjects(t, data) {
		if decoded := decodeStream(t, body); strings.Contains(decoded, "begincmap") {
			return decoded
		}
	}
	t.Fatal("no ToUnicode CMap")
	return ""
}

// bfcharEntries parses the bfchar entries of a CMap, keyed by source CID.
func bfcharEntries(cmap string) map[string]string {
	entries := map[string]string{}
	inMap := false
	for _, line := range strings.Split(cmap, "\n") {
		switch {
		case strings.HasSuffix(line, " beginbfchar"):
			inMap = true
		case line == "endbfchar":
			inMap = false
		case inMap:
			fields := strings.Fields(line)
			if len(fields) == 2 {
				entries[strings.Trim(fields[0], "<>")] = strings.Trim(fields[1], "<>")
			}
		}
	}
	return entries
}

// TestToUnicodeMapsEveryEmittedCID guards RV5-35: the CMap skipped the CID of a
// glyph whose source rune is U+0000, so an emitted glyph had no text-layer
// entry. Every emitted CID now has exactly one entry.
func TestToUnicodeMapsEveryEmittedCID(t *testing.T) {
	f := loadFont(t)
	gid, ok := f.GlyphIndex('A')
	if !ok {
		t.Skip("the test font has no A")
	}
	data := textDocument(t, f, Glyph{GID: gid, Rune: 0}, Glyph{GID: gid, Rune: 'A'})
	entries := bfcharEntries(toUnicodeOf(t, data))
	if len(entries) != 2 {
		t.Fatalf("ToUnicode has %d bfchar entries, want one per emitted CID (2): %v", len(entries), entries)
	}
	if got := entries["0000"]; got != "0000" {
		t.Fatalf("CID 0 maps to %q, want the NUL rune mapped to 0000", got)
	}
	if got := entries["0001"]; got != "0041" {
		t.Fatalf("CID 1 maps to %q, want 0041", got)
	}
	// The page addresses both CIDs, so both entries are needed.
	content := ""
	for _, body := range pdfObjects(t, data) {
		if decoded := decodeStream(t, body); strings.Contains(decoded, "TJ") {
			content += decoded
		}
	}
	if !strings.Contains(content, "<00000001>") {
		t.Fatalf("content does not address both CIDs: %q", content)
	}
}

// TestToUnicodeOrdinaryTextIsUnchanged pins the byte-for-byte output of a run
// without a NUL rune: including U+0000 must not move any ordinary mapping.
func TestToUnicodeOrdinaryTextIsUnchanged(t *testing.T) {
	f := loadFont(t)
	gids := make([]uint16, 2)
	runes := []rune{'A', 'B'}
	for i, r := range runes {
		gid, ok := f.GlyphIndex(r)
		if !ok {
			t.Skipf("the test font has no %c", r)
		}
		gids[i] = gid
	}
	if gids[0] > gids[1] {
		gids[0], gids[1] = gids[1], gids[0]
		runes[0], runes[1] = runes[1], runes[0]
	}
	data := textDocument(t, f, Glyph{GID: gids[0], Rune: runes[0]}, Glyph{GID: gids[1], Rune: runes[1]})
	want := fmt.Sprintf(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
2 beginbfchar
<0000> <%s>
<0001> <%s>
endbfchar
endcmap
CMapName currentdict /CMap defineresource pop
end
end
`, utf16Hex(runes[0]), utf16Hex(runes[1]))
	if got := toUnicodeOf(t, data); got != want {
		t.Fatalf("ordinary ToUnicode changed:\n got: %q\nwant: %q", got, want)
	}
}

var errCompressorFailed = errors.New("compressor failed")

// fakeCompressor is the injected compressor of the streamObject seam.
type fakeCompressor struct {
	writeErr error
	closeErr error
	closed   int
}

func (c *fakeCompressor) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return len(p), nil
}

func (c *fakeCompressor) Close() error {
	c.closed++
	return c.closeErr
}

// streamParts splits a stream object into its /Length value and stream body.
func streamParts(t *testing.T, object []byte) (length int, stream []byte) {
	t.Helper()
	const lengthKey = "/Length "
	i := bytes.Index(object, []byte(lengthKey))
	if i < 0 {
		t.Fatalf("no /Length in %q", object)
	}
	fields := strings.Fields(string(object[i+len(lengthKey):]))
	if len(fields) == 0 {
		t.Fatalf("no /Length value in %q", object)
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("/Length %q is not a number", fields[0])
	}
	start := bytes.Index(object, []byte("stream\n"))
	end := bytes.LastIndex(object, []byte("\nendstream"))
	if start < 0 || end < start {
		t.Fatalf("no stream body in %q", object)
	}
	return n, object[start+len("stream\n") : end]
}

// TestStreamObjectUncompressedOnCompressorFailure guards RV5-36 through the
// injectable compressor: a write or close error falls back to the original
// bytes, keeps /Length correct and closes the compressor exactly once.
func TestStreamObjectUncompressedOnCompressorFailure(t *testing.T) {
	data := bytes.Repeat([]byte("payload "), 32)
	for _, tc := range []struct {
		name     string
		writeErr error
		closeErr error
	}{
		{"write", errCompressorFailed, nil},
		{"close", nil, errCompressorFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCompressor{writeErr: tc.writeErr, closeErr: tc.closeErr}
			body := streamObjectWith(data, true, func(io.Writer) io.WriteCloser { return fake })
			if bytes.Contains(body, []byte("/Filter")) {
				t.Fatalf("a failed compressor still selected a filter:\n%s", body)
			}
			if fake.closed != 1 {
				t.Fatalf("the compressor was closed %d times, want exactly 1", fake.closed)
			}
			length, stream := streamParts(t, body)
			if length != len(data) {
				t.Fatalf("/Length = %d, want the original %d", length, len(data))
			}
			if !bytes.Equal(stream, data) {
				t.Fatalf("stream bytes = %q, want the original data", stream)
			}
		})
	}
}

// TestStreamObjectCompressesWhenSmaller guards the success path: compressible
// data keeps /Filter /FlateDecode and an /Length equal to the compressed body,
// while data that does not shrink is written uncompressed.
func TestStreamObjectCompressesWhenSmaller(t *testing.T) {
	data := bytes.Repeat([]byte("compressible stream payload "), 64)
	body := streamObject(data, true)
	if !bytes.Contains(body, []byte("/Filter /FlateDecode")) {
		t.Fatalf("compressible data was not compressed:\n%s", body)
	}
	length, stream := streamParts(t, body)
	if length != len(stream) {
		t.Fatalf("/Length = %d, compressed body is %d bytes", length, len(stream))
	}
	if len(stream) >= len(data) {
		t.Fatalf("compressed body is %d bytes, input is %d", len(stream), len(data))
	}
	zr, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("the compressed body does not decode to the input")
	}
	// Incompressible data must stay uncompressed with its own length.
	noise := make([]byte, 4096)
	rnd := rand.New(rand.NewSource(1))
	for i := range noise {
		noise[i] = byte(rnd.Intn(256))
	}
	body = streamObject(noise, true)
	if bytes.Contains(body, []byte("/Filter")) {
		t.Fatalf("incompressible data was compressed:\n%.80s", body)
	}
	length, stream = streamParts(t, body)
	if length != len(noise) || !bytes.Equal(stream, noise) {
		t.Fatalf("incompressible body has /Length %d and %d bytes, want %d", length, len(stream), len(noise))
	}
}

// FuzzStreamObject exercises close-on-every-path with arbitrary stream data:
// the object must always declare its real length, and a compressed body must
// decode back to the input.
func FuzzStreamObject(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	f.Add([]byte{0x00, 0xFF, 0x80, 0x13, 0x37, 0x42, 0x00, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		body := streamObject(data, true)
		length, stream := streamParts(t, body)
		if length != len(stream) {
			t.Fatalf("/Length = %d, body is %d bytes", length, len(stream))
		}
		if !bytes.Contains(body, []byte("/Filter /FlateDecode")) {
			if !bytes.Equal(stream, data) {
				t.Fatal("an uncompressed body differs from the input")
			}
			return
		}
		zr, err := zlib.NewReader(bytes.NewReader(stream))
		if err != nil {
			t.Fatalf("compressed stream does not open: %v", err)
		}
		got, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("compressed stream does not decode: %v", err)
		}
		if err := zr.Close(); err != nil {
			t.Fatalf("closing the decoder: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatal("compressed body does not decode to the input")
		}
	})
}
