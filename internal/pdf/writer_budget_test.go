package pdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// writerTestFont loads the bundled font, which keeps the glyph ids and the
// output bytes identical on every platform.
func writerTestFont(t *testing.T) *font.Font {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// writerDoc builds a document whose pages carry one text op per page with
// glyphs distinct glyph-rune pairs, so the writer's CID and glyph work grows
// with pages.
func writerDoc(f *font.Font, pages, glyphs int) *Doc {
	d := New(Options{})
	run := make([]Glyph, glyphs)
	for i := range run {
		run[i] = Glyph{GID: uint16(20 + i%4000), Rune: rune(0x410 + i%4000), Advance: 500}
	}
	for i := 0; i < pages; i++ {
		d.AddPage(595276, 841890, []Op{{
			Kind: TextOpKind,
			Text: &Text{X: 1000, Y: 1000, Font: f, Size: 12000, Glyphs: run},
		}})
	}
	return d
}

// TestSerialisationIsChargedByBytes guards RV4-06: the writer charges the
// bytes it serialises and the glyphs and CIDs it writes, so the recorded spend
// grows with the output instead of staying at a few units per page.
func TestSerialisationIsChargedByBytes(t *testing.T) {
	f := writerTestFont(t)
	spend := func(pages int) (uint64, int) {
		g := budget.New(context.Background(), 0)
		d := writerDoc(f, pages, 2000)
		d.opts.Budget = g
		out, err := d.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return g.Used(), len(out)
	}
	smallSpend, smallOut := spend(1)
	largeSpend, largeOut := spend(8)
	if smallSpend < uint64(smallOut)/4096 {
		t.Fatalf("a %d-byte output spent only %d units", smallOut, smallSpend)
	}
	// Every extra page charges the glyphs it serialises; the font's subset,
	// /W and ToUnicode cost is shared, so the growth is linear after the first
	// page rather than eight times the whole spend.
	if grew := largeSpend - smallSpend; grew < 7*2000/2 {
		t.Fatalf("seven extra pages added only %d units: the glyphs are not charged", grew)
	}
	if largeOut <= smallOut {
		t.Fatalf("the output did not grow with the pages: %d against %d bytes", largeOut, smallOut)
	}
}

// TestSmallBudgetStopsSerialisation guards the second half of RV4-06: a small
// MaxOperations stops the conversion before the output is assembled.
func TestSmallBudgetStopsSerialisation(t *testing.T) {
	f := writerTestFont(t)
	g := budget.New(context.Background(), 2000)
	d := writerDoc(f, 40, 2000)
	d.opts.Budget = g
	out, err := d.Bytes()
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the small budget did not stop the writer: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("the stopped writer returned %d bytes", len(out))
	}
	if used := g.Used(); used > 2000*4 {
		t.Fatalf("the stopped writer spent %d units against a 2000-unit budget", used)
	}
}

// TestZeroAreaImageIsOmitted guards RV4-53: an image whose width or height is
// not positive was written with a zero scale, which is a non-invertible
// transformation matrix. The operation is skipped.
func TestZeroAreaImageIsOmitted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adversarial", "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	img := &Image{Data: data, Format: "png", PixelW: 8, PixelH: 8}
	d := New(Options{})
	d.AddPage(595276, 841890, []Op{
		{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 5000, H: 0}, Image: img},
		{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 0, H: 5000}, Image: img},
		{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 1000, W: 5000, H: 5000}, Image: img},
	})
	out, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	streams := contentStreams(t, out)
	if strings.Count(streams, " cm /") != 1 {
		t.Fatalf("the page writes %d image operations, want 1:\n%s", strings.Count(streams, " cm /"), streams)
	}
}

// contentStreams joins the content streams of a PDF for inspection.
func contentStreams(t *testing.T, data []byte) string {
	t.Helper()
	var b strings.Builder
	for _, stream := range pdfdebug.ContentStreams(data) {
		b.Write(stream)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestWideWidthArrayIsSplit guards RV4-52: a subset with thousands of distinct
// widths wrote one array, which a strict viewer may reject. /W is written as
// several startCID groups, none larger than the compatibility limit.
func TestWideWidthArrayIsSplit(t *testing.T) {
	f := writerTestFont(t)
	fe := &fontEntry{font: f, cidGID: make([]uint16, 9000)}
	for i := range fe.cidGID {
		fe.cidGID[i] = uint16(i % int(f.NumGlyphs()))
	}
	w, err := fe.widthArray(nil)
	if err != nil {
		t.Fatal(err)
	}
	groups := splitGroups(w)
	if len(groups) < 2 {
		t.Fatalf("/W has %d groups, want several (head %q)", len(groups), w[:min(len(w), 40)])
	}
	total := 0
	for i, group := range groups {
		if len(group) > maxArrayElements {
			t.Fatalf("group %d holds %d widths, the limit is %d", i, len(group), maxArrayElements)
		}
		total += len(group)
	}
	if total != len(fe.cidGID) {
		t.Fatalf("the groups describe %d widths, want %d", total, len(fe.cidGID))
	}
	// The start CID of each group matches its place in the array.
	rest := w
	want := 0
	for _, group := range groups {
		i := strings.IndexByte(rest, '[')
		if i < 0 {
			break
		}
		fields := strings.Fields(rest[:i])
		if len(fields) == 0 || fields[len(fields)-1] != strconv.Itoa(want) {
			t.Fatalf("the group start is %v, want %d", fields, want)
		}
		want += len(group)
		rest = rest[i+1:]
		j := strings.IndexByte(rest, ']')
		if j < 0 {
			break
		}
		rest = rest[j+1:]
	}
}

// splitGroups parses "/W" groups: a start CID followed by a bracketed list.
func splitGroups(w string) [][]string {
	var groups [][]string
	for {
		i := strings.IndexByte(w, '[')
		if i < 0 {
			break
		}
		j := strings.IndexByte(w[i:], ']')
		if j < 0 {
			break
		}
		groups = append(groups, strings.Fields(w[i+1:i+j]))
		w = w[i+j+1:]
	}
	return groups
}

// TestBaseFontNameStaysWithinTheLimit guards RV4-54: a long family name made
// /BaseFont exceed the 127-byte architectural limit. The name is capped and
// every subset keeps a distinct tag.
func TestBaseFontNameStaysWithinTheLimit(t *testing.T) {
	long := strings.Repeat("Very-Long-Family-Name-", 10)
	a := baseFontName("ABCDEF", long, "Bold")
	b := baseFontName("GHIJKL", long, "Bold")
	for _, name := range []string{a, b} {
		if len(name) > maxBaseFontBytes {
			t.Fatalf("the base font name is %d bytes: %q", len(name), name)
		}
	}
	if a == b {
		t.Fatal("two subsets share one name")
	}
	if !strings.HasPrefix(a, "ABCDEF+") {
		t.Fatalf("the subset tag is missing: %q", a)
	}
	short := baseFontName("ABCDEF", "DejaVu Sans", "Book")
	if short != "ABCDEF+DejaVu-Sans-Book" {
		t.Fatalf("a short name was changed: %q", short)
	}
}

// TestPNGKeepsColourTypeAndDepth guards RV4-24: every PNG was written as
// 8-bit DeviceRGB, so a 16-bit grayscale source lost its low-order bits and a
// 1-bit drawing became 24-bit RGB. The source colour type and bit depth are
// preserved.
func TestPNGKeepsColourTypeAndDepth(t *testing.T) {
	// A 16-bit grayscale source with a distinguishable low byte.
	g16 := image.NewGray16(image.Rect(0, 0, 2, 1))
	g16.SetGray16(0, 0, color.Gray16{Y: 0x1234})
	g16.SetGray16(1, 0, color.Gray16{Y: 0xABCD})
	var buf bytes.Buffer
	if err := png.Encode(&buf, g16); err != nil {
		t.Fatal(err)
	}
	body := imageObjectFor(t, buf.Bytes())
	if !strings.Contains(body, "/ColorSpace /DeviceGray") || !strings.Contains(body, "/BitsPerComponent 16") {
		t.Fatalf("the 16-bit grayscale image was not preserved:\n%s", body)
	}
	samples := imageSamples(t, body)
	if len(samples) != 4 || samples[0] != 0x12 || samples[1] != 0x34 || samples[2] != 0xAB || samples[3] != 0xCD {
		t.Fatalf("the 16-bit samples are % x, want 12 34 ab cd", samples)
	}

	// A one-bit grayscale drawing stays one bit per pixel in gray.
	oneBit := oneBitPNG(8, 1)
	body = imageObjectFor(t, oneBit)
	if !strings.Contains(body, "/ColorSpace /DeviceGray") || !strings.Contains(body, "/BitsPerComponent 1") {
		t.Fatalf("the one-bit image was expanded:\n%s", body)
	}
	if samples := imageSamples(t, body); len(samples) != 1 {
		t.Fatalf("the one-bit row is %d bytes, want 1", len(samples))
	}
}

// oneBitPNG builds a one-bit grayscale PNG with a horizontally split pattern.
func oneBitPNG(w, h int) []byte {
	var raw bytes.Buffer
	rowBytes := (w + 7) / 8
	for y := 0; y < h; y++ {
		raw.WriteByte(0) // filter: none
		for i := 0; i < rowBytes; i++ {
			raw.WriteByte(0xF0)
		}
	}
	var idat bytes.Buffer
	zw := zlib.NewWriter(&idat)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	chunk := func(tag string, data []byte) []byte {
		out := make([]byte, 0, len(data)+12)
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(data)))
		out = append(out, length[:]...)
		out = append(out, tag...)
		out = append(out, data...)
		crc := crc32.NewIEEE()
		crc.Write([]byte(tag))
		crc.Write(data)
		var sum [4]byte
		binary.BigEndian.PutUint32(sum[:], crc.Sum32())
		return append(out, sum[:]...)
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(h))
	ihdr[8] = 1  // bit depth
	ihdr[9] = 0  // grayscale
	ihdr[10] = 0 // deflate
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // no interlace
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, chunk("IHDR", ihdr)...)
	out = append(out, chunk("IDAT", idat.Bytes())...)
	out = append(out, chunk("IEND", nil)...)
	return out
}

// imageObjectFor embeds one PNG and returns its image XObject body.
func imageObjectFor(t *testing.T, data []byte) string {
	t.Helper()
	obj, err := imageObject(nil, &Image{Data: data, Format: "png"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(obj)
}

// imageSamples returns the decoded pixel bytes of an image stream body.
func imageSamples(t *testing.T, body string) []byte {
	t.Helper()
	i := strings.Index(body, "stream\n")
	j := strings.LastIndex(body, "\nendstream")
	if i < 0 || j < i {
		t.Fatalf("no stream in %q", body)
	}
	raw := []byte(body[i+len("stream\n") : j])
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestDecodedPNGIsReused guards RV4-66: the writer decoded a PNG the loader had
// already decoded. When the image carries its decoded form, the writer must use
// it — here the encoded body is truncated, so a second decode would fail.
func TestDecodedPNGIsReused(t *testing.T) {
	full := oneBitPNG(8, 1)
	header := full[:33] // the signature and the IHDR chunk only
	decoded := image.NewGray(image.Rect(0, 0, 8, 1))
	for i := 0; i < 8; i++ {
		decoded.SetGray(i, 0, color.Gray{Y: 0xFF})
	}
	obj, err := imageObject(nil, &Image{Data: header, Format: "png", Decoded: decoded}, nil)
	if err != nil {
		t.Fatalf("the writer decoded the body again: %v", err)
	}
	if !strings.Contains(string(obj), "/Subtype /Image") {
		t.Fatal("no image object")
	}
}
