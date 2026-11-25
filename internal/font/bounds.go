package font

import "encoding/binary"

// readU32 reads a big-endian uint32 at off. It reports false when off is
// outside data, and returns the value as uint64: a font-declared offset above
// 0x7FFFFFFF becomes negative when converted to int on a 32-bit build, so the
// caller must compare it with a table length before any int conversion.
func readU32(data []byte, off int) (uint64, bool) {
	if off < 0 || uint64(off)+4 > uint64(len(data)) {
		return 0, false
	}
	return uint64(binary.BigEndian.Uint32(data[off : off+4])), true
}

// fits reports whether the range [base+delta, base+delta+size) is inside n
// bytes. The arithmetic is uint64, so a value that does not fit an int on the
// current architecture is rejected instead of wrapping into a negative index.
func fits(base int, delta, size uint64, n int) bool {
	if base < 0 || n < 0 {
		return false
	}
	return uint64(base)+delta+size <= uint64(n)
}
