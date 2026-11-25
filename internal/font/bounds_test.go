package font

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"testing"
)

// cmapWithOffset builds a cmap with one encoding record whose subtable offset
// is the given value. The table itself stops at the record: nothing is stored
// at the offset, so a parser must reject it on the offset alone.
func cmapWithOffset(offset uint32) []byte {
	cmap := make([]byte, 12)
	binary.BigEndian.PutUint16(cmap[2:4], 1) // one encoding record
	binary.BigEndian.PutUint16(cmap[4:6], 3)
	binary.BigEndian.PutUint16(cmap[6:8], 1)
	binary.BigEndian.PutUint32(cmap[8:12], offset)
	return cmap
}

// TestReadU32AndFitsRejectHighValues guards the helpers every font offset goes
// through: a value above MaxInt32 must be refused before it becomes a negative
// index on a 32-bit build.
func TestReadU32AndFitsRejectHighValues(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	if v, ok := readU32(data, 0); !ok || v != 0x01020304 {
		t.Fatalf("readU32 = %#x, %v", v, ok)
	}
	if _, ok := readU32(data, 1); ok {
		t.Fatal("readU32 accepted a four-byte read past the end")
	}
	if fits(0, 0x80000000, 0, 100) {
		t.Fatal("fits accepted 0x80000000 inside a 100-byte table")
	}
	if !fits(0, 4, 4, 8) {
		t.Fatal("fits rejected a range that is exactly inside the table")
	}
}

// TestCmapOffsetAboveMaxIntIsRejected guards the 32-bit path: as an int,
// 0x80000000 is negative, so the subtable offset used to be sliced before the
// bounds check could reject it.
func TestCmapOffsetAboveMaxIntIsRejected(t *testing.T) {
	f := &Font{tables: map[string][]byte{"cmap": cmapWithOffset(0x80000000)}}
	if err := f.parseCmap(); err == nil {
		t.Fatal("an out-of-table cmap offset was accepted")
	}
}

// TestCmapFormat12GroupAboveTheGlyphSpace guards the format 12 arithmetic: the
// sum startGlyphID + span wraps in uint32 and used to pass the check, mapping
// characters to glyphs outside the 16-bit space.
func TestCmapFormat12GroupAboveTheGlyphSpace(t *testing.T) {
	// 0xFFFFFFF0 plus the 17-character span wraps to a small number in uint32,
	// which is why the comparison must be 64-bit.
	over := cmap12Test([3]uint32{'A', 'A' + 0x10, 0xFFFFFFF0})
	if _, err := parseCmapSubtable(over, 0); err == nil {
		t.Fatal("a format 12 group that wraps past 0xFFFF was accepted")
	}
	// A group that ends exactly at the last glyph id is still valid.
	ok := cmap12Test([3]uint32{0x100, 0x10F, 0xFFF0})
	if _, err := parseCmapSubtable(ok, 0); err != nil {
		t.Fatalf("a valid group at the top of the glyph space was rejected: %v", err)
	}
}

// TestCFFPrivateDictOffsetsDoNotOverflow guards the CFF DICT offsets: size and
// offset are int32 operands, so their sum must be widened before the slice.
func TestCFFPrivateDictOffsetsDoNotOverflow(t *testing.T) {
	dict := cffDict{cffOpPrivate: []cffDictValue{
		{integer: 16},
		{integer: math.MaxInt32},
	}}
	if _, err := cffFontDictAt(make([]byte, 64), dict); err == nil {
		t.Fatal("a Private DICT outside the table was accepted")
	}
}

// TestCFFIndexRejectsOverlongOffsets guards the INDEX reader: its offsets are
// read and added in int64, so a four-byte value above MaxInt32 cannot become a
// negative index.
func TestCFFIndexRejectsOverlongOffsets(t *testing.T) {
	data := []byte{
		0, 1, // one entry
		4,             // four-byte offsets
		0xFF, 0, 0, 0, // first offset
		0xFF, 0, 0, 1, // second offset
		0, // one data byte
	}
	if _, _, err := cffIndex(data, 0); err == nil {
		t.Fatal("an out-of-table INDEX offset was accepted")
	}
}

// TestPackageBuildsFor386 guards the 32-bit build: the package must compile
// where int is 32 bits, which is where an offset above MaxInt32 would wrap.
func TestPackageBuildsFor386(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go tool is required to run the tests: %v", err)
	}
	cmd := exec.Command(goTool, "build", "-mod=readonly", "./")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=386", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the font package does not build for linux/386: %v\n%s", err, out)
	}
}
