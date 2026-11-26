package pdf

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

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
