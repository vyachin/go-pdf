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
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// imageObjectAndMask embeds one PNG and returns the image XObject together with
// the soft-mask stream the writer allocated for it.
func imageObjectAndMask(t *testing.T, data []byte) (body string, mask []byte) {
	t.Helper()
	obj, err := imageObject(nil, &Image{Data: data, Format: "png", PixelW: 1, PixelH: 1}, func(m []byte) int {
		mask = append([]byte(nil), m...)
		return 7
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(obj), mask
}

// encodeTestPNG encodes an image with the standard encoder.
func encodeTestPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// palettePNG encodes one row of palette entries as an 8-bit indexed PNG. The
// encoder writes a tRNS chunk when an entry is not opaque.
func palettePNG(t *testing.T, entries ...color.NRGBA) []byte {
	t.Helper()
	palette := make(color.Palette, len(entries))
	for i, entry := range entries {
		palette[i] = entry
	}
	img := image.NewPaletted(image.Rect(0, 0, len(entries), 1), palette)
	for i := range entries {
		img.Pix[i] = uint8(i)
	}
	return encodeTestPNG(t, img)
}

// grayAlphaPNG builds a one-pixel grayscale+alpha PNG, colour type 4. The
// standard encoder has no gray+alpha image type, so the chunks are written here.
func grayAlphaPNG(t *testing.T, gray, alpha uint8) []byte {
	t.Helper()
	var idat bytes.Buffer
	zw := zlib.NewWriter(&idat)
	// One scanline: filter type 0, then the gray and alpha samples.
	if _, err := zw.Write([]byte{0, gray, alpha}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], 1)
	binary.BigEndian.PutUint32(ihdr[4:8], 1)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 4 // grayscale plus alpha
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, pngChunk("IHDR", ihdr)...)
	out = append(out, pngChunk("IDAT", idat.Bytes())...)
	out = append(out, pngChunk("IEND", nil)...)
	return out
}

// pngChunk writes one PNG chunk with its length and CRC.
func pngChunk(tag string, data []byte) []byte {
	out := make([]byte, 0, len(data)+12)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	out = append(out, length[:]...)
	out = append(out, tag...)
	out = append(out, data...)
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(tag))
	_, _ = crc.Write(data)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	return append(out, sum[:]...)
}

// TestTransparentPNGColourIsUnpremultiplied guards RV5-33: palette and
// gray+alpha samples were written premultiplied and then multiplied again by the
// soft mask. The colour stream must carry the source colour while the mask keeps
// the alpha.
func TestTransparentPNGColourIsUnpremultiplied(t *testing.T) {
	cases := []struct {
		name       string
		png        []byte
		color      []byte
		mask       []byte
		colorSpace string
	}{
		{
			name:       "palette red at half alpha",
			png:        palettePNG(t, color.NRGBA{R: 255, A: 128}),
			color:      []byte{0xFF, 0x00, 0x00},
			mask:       []byte{0x80},
			colorSpace: "/DeviceRGB",
		},
		{
			name:       "gray 128 at alpha 128",
			png:        grayAlphaPNG(t, 128, 128),
			color:      []byte{0x80},
			mask:       []byte{0x80},
			colorSpace: "/DeviceGray",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, mask := imageObjectAndMask(t, tc.png)
			if !strings.Contains(body, "/ColorSpace "+tc.colorSpace) {
				t.Fatalf("image object has the wrong colour space:\n%s", body)
			}
			if !strings.Contains(body, "/SMask 7 0 R") {
				t.Fatalf("image object has no soft mask reference:\n%s", body)
			}
			if got := imageSamples(t, body); !bytes.Equal(got, tc.color) {
				t.Fatalf("colour samples = % x, want % x", got, tc.color)
			}
			if got := imageSamples(t, string(mask)); !bytes.Equal(got, tc.mask) {
				t.Fatalf("mask samples = % x, want % x", got, tc.mask)
			}
		})
	}
}

// TestOpaquePNGColourIsUnchanged guards the control case: when every pixel is
// opaque, a palette or NRGBA source keeps its bytes and carries no soft mask.
func TestOpaquePNGColourIsUnchanged(t *testing.T) {
	opaque := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	opaque.SetNRGBA(0, 0, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})
	tests := []struct {
		name string
		png  []byte
	}{
		{"palette", palettePNG(t, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})},
		{"nrgba", encodeTestPNG(t, opaque)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, mask := imageObjectAndMask(t, tc.png)
			if mask != nil {
				t.Fatalf("an opaque image allocated a soft mask: % x", mask)
			}
			if strings.Contains(body, "/SMask") {
				t.Fatalf("an opaque image references a soft mask:\n%s", body)
			}
			if got := imageSamples(t, body); !bytes.Equal(got, []byte{0x12, 0x34, 0x56}) {
				t.Fatalf("colour samples = % x, want 12 34 56", got)
			}
		})
	}
}

func TestGraySamplesPackRowsIndependently(t *testing.T) {
	// Three pixels leave padding at every supported sub-byte depth. The
	// second row must start at a new byte, not inside the first row's padding.
	img := image.NewGray(image.Rect(10, 20, 13, 22))
	copy(img.Pix, []byte{0, 255, 0, 255, 0, 255})
	for _, tc := range []struct {
		bits int
		want []byte
	}{
		{1, []byte{0x40, 0xa0}},
		{2, []byte{0x30, 0xcc}},
		{4, []byte{0x0f, 0x00, 0xf0, 0xf0}},
		{8, []byte{0, 255, 0, 255, 0, 255}},
	} {
		t.Run(strconv.Itoa(tc.bits), func(t *testing.T) {
			got, alpha := graySamples(tc.bits, 3, 2, img)
			if !bytes.Equal(got, tc.want) || alpha != nil {
				t.Fatalf("samples = %x, alpha = %x; want %x without a mask", got, alpha, tc.want)
			}
		})
	}
}

func TestGraySamplesRespectSubimageStride(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 5, 4))
	img.SetGray(2, 1, color.Gray{Y: 0x12})
	img.SetGray(3, 1, color.Gray{Y: 0x34})
	img.SetGray(2, 2, color.Gray{Y: 0x56})
	img.SetGray(3, 2, color.Gray{Y: 0x78})
	got, alpha := graySamples(8, 2, 2, img.SubImage(image.Rect(2, 1, 4, 3)))
	if !bytes.Equal(got, []byte{0x12, 0x34, 0x56, 0x78}) || alpha != nil {
		t.Fatalf("cropped gray samples = %x, mask = %x", got, alpha)
	}
	img16 := image.NewGray16(image.Rect(0, 0, 5, 4))
	img16.SetGray16(2, 1, color.Gray16{Y: 0x1234})
	img16.SetGray16(3, 1, color.Gray16{Y: 0x5678})
	img16.SetGray16(2, 2, color.Gray16{Y: 0x9abc})
	img16.SetGray16(3, 2, color.Gray16{Y: 0xdef0})
	got, alpha = graySamples(16, 2, 2, img16.SubImage(image.Rect(2, 1, 4, 3)))
	if !bytes.Equal(got, []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}) || alpha != nil {
		t.Fatalf("cropped 16-bit samples = %x, mask = %x", got, alpha)
	}
}

func TestRGBSamplesKeepUnassociatedColorAndAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.SetNRGBA(1, 1, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0x80})
	img.SetNRGBA(2, 1, color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0xff})
	got, alpha := rgbSamples(8, 2, 1, img.SubImage(image.Rect(1, 1, 3, 2)))
	if !bytes.Equal(got, []byte{0x12, 0x34, 0x56, 0xab, 0xcd, 0xef}) || !bytes.Equal(alpha, []byte{0x80, 0xff}) {
		t.Fatalf("8-bit samples = %x, mask = %x", got, alpha)
	}
	img16 := image.NewNRGBA64(image.Rect(0, 0, 4, 3))
	img16.SetNRGBA64(1, 1, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0x80ff})
	img16.SetNRGBA64(2, 1, color.NRGBA64{R: 0xdef0, G: 0x2468, B: 0x1357, A: 0xffff})
	got, alpha = rgbSamples(16, 2, 1, img16.SubImage(image.Rect(1, 1, 3, 2)))
	if !bytes.Equal(got, []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x24, 0x68, 0x13, 0x57}) || !bytes.Equal(alpha, []byte{0x80, 0xff}) {
		t.Fatalf("16-bit samples = %x, mask = %x", got, alpha)
	}
}

func TestPNGPaletteExpandsToRGB(t *testing.T) {
	img := image.NewPaletted(image.Rect(4, 5, 7, 6), color.Palette{
		color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}, color.RGBA{B: 255, A: 255},
	})
	copy(img.Pix, []byte{2, 0, 1})
	got, err := pngToSamples(pngHeader{width: 3, height: 1, bitDepth: 2, colorType: 3}, img, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.colorSpace != "/DeviceRGB" || got.bits != 8 || got.width != 3 || got.height != 1 ||
		!bytes.Equal(got.color, []byte{0, 0, 255, 255, 0, 0, 0, 255, 0}) || got.alpha != nil {
		t.Fatalf("palette samples = %+v", got)
	}
}

func TestPNGSampleLimits(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3, 2))
	h := pngHeader{width: 3, height: 2, bitDepth: 8, colorType: 0}
	if got, err := pngToSamples(h, img, budget.New(context.Background(), 5)); got != nil || !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("six pixels under a five-unit budget = %+v, %v", got, err)
	}
	got, err := pngToSamples(h, img, budget.New(context.Background(), 6))
	if err != nil || got == nil || got.colorSpace != "/DeviceGray" || got.bits != 8 || len(got.color) != 6 {
		t.Fatalf("six pixels under a six-unit budget = %+v, %v", got, err)
	}
	h.bitDepth = 3
	if got, err := pngToSamples(h, img, nil); got != nil || err == nil {
		t.Fatalf("unsupported bit depth = %+v, %v", got, err)
	}
}

func TestDecodedPNGRejectsDamagedInput(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	img, h, err := decodedPNG(data, nil)
	if err != nil || img == nil || h != (pngHeader{width: 3, height: 2, bitDepth: 8, colorType: 0}) {
		t.Fatalf("decodedPNG = %v, %+v, %v", img, h, err)
	}
	badCRC := append([]byte(nil), data...)
	badCRC[29] ^= 0xff
	for _, damaged := range [][]byte{nil, []byte("not a PNG"), data[:32], data[:33], badCRC} {
		img, _, err := decodedPNG(damaged, nil)
		if err == nil || img != nil {
			t.Errorf("damaged PNG decoded as %v, %v", img, err)
		}
	}
}
