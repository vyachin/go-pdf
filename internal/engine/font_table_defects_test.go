package engine

import (
	"context"
	"encoding/binary"
	"os"
	"slices"
	"strings"
	"testing"
)

// malformedFontBytes builds a two-glyph TrueType font whose glyph 1 is a
// composite referencing glyph id 2, which is at numGlyphs. The cmap maps 'A'
// to that glyph, so a document reaches it through the public conversion path.
func malformedFontBytes() []byte {
	const numGlyphs = 2
	composite := make([]byte, 10)
	binary.BigEndian.PutUint16(composite[0:2], 0xFFFF) // composite
	composite = append(composite, 0, 0x02, 0, 2)       // ARGS_ARE_XY_VALUES, component 2
	composite = append(composite, 0, 0)                // offset (0,0)

	locaVals := []uint32{0, 0, uint32(len(composite))}
	loca := make([]byte, len(locaVals)*4)
	for i, v := range locaVals {
		binary.BigEndian.PutUint32(loca[i*4:i*4+4], v)
	}

	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head[0:4], 0x00010000)
	binary.BigEndian.PutUint32(head[12:16], 0x5F0F3CF5) // magicNumber
	binary.BigEndian.PutUint16(head[18:20], 1000)       // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)          // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint32(hhea[0:4], 0x00010000)
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	hmtx := []byte{0, 100, 0, 0}

	// cmap format 12: one group mapping 'A' to glyph 1.
	sub := make([]byte, 28)
	binary.BigEndian.PutUint16(sub[0:2], 12)
	binary.BigEndian.PutUint32(sub[4:8], uint32(len(sub)))
	binary.BigEndian.PutUint32(sub[12:16], 1) // one group
	binary.BigEndian.PutUint32(sub[16:20], 'A')
	binary.BigEndian.PutUint32(sub[20:24], 'A')
	binary.BigEndian.PutUint32(sub[24:28], 1) // glyph 1
	cmap := make([]byte, 12)
	binary.BigEndian.PutUint16(cmap[2:4], 1)  // one encoding record
	binary.BigEndian.PutUint16(cmap[4:6], 3)  // Windows
	binary.BigEndian.PutUint16(cmap[6:8], 10) // Unicode full repertoire
	binary.BigEndian.PutUint32(cmap[8:12], 12)
	cmap = append(cmap, sub...)

	tables := map[string][]byte{
		"cmap": cmap,
		"glyf": composite,
		"head": head,
		"hhea": hhea,
		"hmtx": hmtx,
		"loca": loca,
		"maxp": maxp,
	}
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	out := make([]byte, 12+16*len(tags))
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(tags)))
	offset := len(out)
	for i, tag := range tags {
		rec := 12 + i*16
		copy(out[rec:rec+4], tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(offset))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(tables[tag])))
		out = append(out, tables[tag]...)
		offset += len(tables[tag])
		for len(out)%4 != 0 {
			out = append(out, 0)
			offset++
		}
	}
	return out
}

// TestDanglingComponentReachesTheUser guards RV5-30 through the public
// boundary: a document-supplied font with a dangling component makes the
// conversion fail with a diagnostic instead of writing a PDF with a broken
// reference, and it does not panic.
func TestDanglingComponentReachesTheUser(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/broken.ttf"
	if err := os.WriteFile(path, malformedFontBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`<style>@font-face { font-family: broken; src: url(broken.ttf) }
		p { font-family: broken; font-size: 12pt }</style><p>AAAA</p>`)
	res, err := Convert(context.Background(), src, Options{BaseDir: dir, File: "broken.html"})
	if err == nil {
		t.Fatal("the conversion wrote a PDF with a dangling component")
	}
	if len(res.PDF) != 0 {
		t.Fatalf("the failed conversion returned %d PDF bytes", len(res.PDF))
	}
	if len(res.Diagnostics) == 0 {
		t.Fatal("the failure produced no diagnostic")
	}
	lines := strings.Join(res.Diagnostics.Lines(), "\n")
	if !strings.Contains(lines, "component 2") {
		t.Fatalf("the diagnostic does not name the dangling component:\n%s", lines)
	}
}
