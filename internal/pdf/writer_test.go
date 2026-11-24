package pdf

import (
	"bytes"
	"compress/zlib"
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
