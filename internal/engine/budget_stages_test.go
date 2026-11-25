package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/budget"
)

// writeSizedPNG writes a solid PNG of the given pixel size. The decoding charge
// grows with the pixel count, so a test can exhaust a budget without writing
// hundreds of files.
func writeSizedPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 120
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// gposAmplifier builds a GPOS table whose one PairPos lookup declares
// `subtables` subtables pointing at the lookup itself: enough charged work to
// exhaust a small budget without a large file.
func gposAmplifier(subtables int) []byte {
	const lookupList = 10
	lookupOff := lookupList + 2 + 2
	lookupSize := 6 + subtables*2
	out := make([]byte, lookupOff+lookupSize)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[8:10], lookupList)
	binary.BigEndian.PutUint16(out[lookupList:lookupList+2], 1)
	binary.BigEndian.PutUint16(out[lookupList+2:lookupList+4], uint16(lookupOff-lookupList))
	binary.BigEndian.PutUint16(out[lookupOff:lookupOff+2], 2) // PairPos
	binary.BigEndian.PutUint16(out[lookupOff+4:lookupOff+6], uint16(subtables))
	return out
}

// buildTestFont assembles the smallest complete TrueType font carrying gpos.
func buildTestFont(gpos []byte) []byte {
	const numGlyphs = 2
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)    // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800) // ascent
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	hmtx := []byte{0, 100, 0, 0}
	loca := make([]byte, (numGlyphs+1)*4)
	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", head}, {"hhea", hhea}, {"maxp", maxp}, {"hmtx", hmtx},
		{"loca", loca}, {"glyf", nil}, {"GPOS", gpos},
	}
	out := make([]byte, 12+len(tables)*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(tables)))
	off := len(out)
	for i, table := range tables {
		rec := 12 + i*16
		copy(out[rec:rec+4], table.tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(off))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(table.data)))
		out = append(out, table.data...)
		off += len(table.data)
		for len(out)%4 != 0 {
			out = append(out, 0)
			off++
		}
	}
	return out
}

// TestCascadeMatchingIsCharged guards the cascade stage: matching many rules
// against many elements must stop at MaxOperations instead of running the full
// rule × element product.
func TestCascadeMatchingIsCharged(t *testing.T) {
	var b strings.Builder
	b.WriteString("<style>")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, ".c%d p { color: #000 }\n", i)
	}
	b.WriteString("</style>")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, `<div class="c%d"><p>text</p></div>`, i)
	}
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		File:         "cascade-budget.html",
		Limits:       Limits{MaxOperations: 20_000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("cascade matching was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestImageDecodingIsCharged guards the image stage: the budget is spent from
// the image header before the decoders run on the images that would cross it.
func TestImageDecodingIsCharged(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	const images = 60
	for i := 0; i < images; i++ {
		name := fmt.Sprintf("i%d.png", i)
		writeSizedPNG(t, filepath.Join(dir, name), 512, 512)
		fmt.Fprintf(&b, `<img src="%s">`, name)
	}
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		BaseDir:      dir,
		File:         "image-budget.html",
		Limits:       Limits{MaxOperations: 5000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("image decoding was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestFontParsingIsCharged guards the font stage end to end: the amplifier
// GPOS exhausts a small budget during prepare, before layout.
func TestFontParsingIsCharged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amplifier.ttf")
	if err := os.WriteFile(path, buildTestFont(gposAmplifier(100)), 0o644); err != nil {
		t.Fatal(err)
	}
	// No text: the crafted font has no cmap, and this test is about the parsing
	// charge, not about glyph lookup.
	res, err := Convert(context.Background(), []byte(`<div style="height: 1pt"></div>`), Options{
		FallbackFont: path,
		File:         "font-budget.html",
		Limits:       Limits{MaxOperations: 250},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("font parsing was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestImageLoaderHonoursCancellation guards that a cancelled context is visible
// before an image is decoded.
func TestImageLoaderHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := &imageLoader{guard: budget.New(ctx, 0)}
	if err := l.charge(1 << 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("image charge returned %v, want context.Canceled", err)
	}
}

// TestCancellationDuringSelectorMatching covers the cancellation scenario: a
// cancelled context with a rule-heavy document returns the context error, not
// a document diagnostic.
func TestCancellationDuringSelectorMatching(t *testing.T) {
	var b strings.Builder
	b.WriteString("<style>")
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&b, ".c%d p { color: #000 }\n", i)
	}
	b.WriteString("</style>")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, `<div class="c%d"><p>x</p></div>`, i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	_, err := Convert(ctx, []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		File:         "cancel-match.html",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Convert returned %v, want context.Canceled", err)
	}
}
