package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/vyachin/go-pdf/internal/budget"
)

// pngHeader is the part of a PNG's IHDR the writer needs: the source colour
// type and bit depth decide its PDF colour space and sample width.
type pngHeader struct {
	width, height int
	bitDepth      int
	colorType     int
}

// readPNGHeader parses the IHDR chunk of a PNG. The first chunk of a valid PNG
// is always IHDR, followed by its CRC.
func readPNGHeader(data []byte) (pngHeader, error) {
	if len(data) < 33 || string(data[:8]) != "\x89PNG\r\n\x1a\n" || string(data[12:16]) != "IHDR" {
		return pngHeader{}, fmt.Errorf("pdf: not a PNG file")
	}
	h := pngHeader{
		width:     int(binary.BigEndian.Uint32(data[16:20])),
		height:    int(binary.BigEndian.Uint32(data[20:24])),
		bitDepth:  int(data[24]),
		colorType: int(data[25]),
	}
	return h, nil
}

// pngSamples is one PNG ready for the PDF image stream: the colour space and
// bit depth of the source, the samples themselves and, when the source has
// transparency, an 8-bit alpha channel for the soft mask.
type pngSamples struct {
	width, height int
	colorSpace    string
	bits          int
	color         []byte
	alpha         []byte
}

// pngToSamples converts a decoded PNG into PDF samples, keeping the source
// colour type and bit depth: a grayscale image stays grayscale, a 1-bit
// drawing stays one bit per pixel and a 16-bit source keeps its low-order
// bits. Only a palette source falls back to 8-bit RGB, because PDF has no
// palette colour space.
func pngToSamples(header pngHeader, decoded image.Image, guard *budget.Guard) (*pngSamples, error) {
	out := &pngSamples{width: header.width, height: header.height}
	gray := header.colorType == 0 || header.colorType == 4
	switch {
	case gray:
		out.colorSpace = "/DeviceGray"
		out.bits = header.bitDepth
	case header.colorType == 2 || header.colorType == 6:
		out.colorSpace = "/DeviceRGB"
		out.bits = header.bitDepth
	default:
		// Palette and anything else: PDF has no indexed space here, so the
		// pixels are written as 8-bit RGB.
		out.colorSpace = "/DeviceRGB"
		out.bits = 8
	}
	if out.bits != 1 && out.bits != 2 && out.bits != 4 && out.bits != 8 && out.bits != 16 {
		return nil, fmt.Errorf("pdf: unsupported PNG bit depth %d", out.bits)
	}
	// One unit per decoded pixel, charged before the output slices exist.
	if err := guard.Add(uint64(header.width) * uint64(header.height)); err != nil {
		return nil, err
	}
	if gray {
		out.color, out.alpha = graySamples(out.bits, out.width, out.height, decoded)
		return out, nil
	}
	out.color, out.alpha = rgbSamples(out.bits, out.width, out.height, decoded)
	return out, nil
}

// graySamples writes one sample per pixel at the requested depth, packing
// depths below eight bits MSB first with each row padded to a byte boundary.
func graySamples(bits, width, height int, img image.Image) (colorBytes, alpha []byte) {
	b := img.Bounds()
	// Typed fast paths: the common colour models are read straight from their
	// pixel slices instead of one At() call per pixel.
	if bits == 8 {
		if m, ok := img.(*image.Gray); ok {
			colorBytes = make([]byte, 0, width*height)
			for y := 0; y < height; y++ {
				row := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y) : m.PixOffset(b.Min.X, b.Min.Y+y)+width]
				colorBytes = append(colorBytes, row...)
			}
			return colorBytes, nil
		}
	}
	if bits == 16 {
		if m, ok := img.(*image.Gray16); ok {
			colorBytes = make([]byte, 0, width*height*2)
			for y := 0; y < height; y++ {
				row := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y) : m.PixOffset(b.Min.X, b.Min.Y+y)+width*2]
				colorBytes = append(colorBytes, row...)
			}
			return colorBytes, nil
		}
	}
	if bits == 16 {
		colorBytes = make([]byte, 0, width*height*2)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				g := color.Gray16Model.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray16)
				colorBytes = append(colorBytes, byte(g.Y>>8), byte(g.Y))
				alpha = append(alpha, byte(alphaOf(img, b.Min.X+x, b.Min.Y+y)))
			}
		}
		return colorBytes, opaqueMask(alpha)
	}
	rowBytes := (width*bits + 7) / 8
	colorBytes = make([]byte, 0, rowBytes*height)
	maxV := (1 << bits) - 1
	for y := 0; y < height; y++ {
		row := make([]byte, rowBytes)
		for x := 0; x < width; x++ {
			g := color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray)
			v := int(g.Y) * maxV / 255
			bit := x * bits
			row[bit/8] |= byte(v << uint(8-bits-bit%8))
			alpha = append(alpha, byte(alphaOf(img, b.Min.X+x, b.Min.Y+y)))
		}
		colorBytes = append(colorBytes, row...)
	}
	return colorBytes, opaqueMask(alpha)
}

// rgbSamples writes three samples per pixel at the requested depth.
func rgbSamples(bits, width, height int, img image.Image) (colorBytes, alpha []byte) {
	b := img.Bounds()
	if bits == 8 {
		switch m := img.(type) {
		case *image.NRGBA:
			// Non-premultiplied 8-bit RGBA: the colour bytes are the first
			// three of every four.
			colorBytes = make([]byte, 0, width*height*3)
			for y := 0; y < height; y++ {
				off := m.PixOffset(b.Min.X, b.Min.Y+y)
				for x := 0; x < width; x++ {
					colorBytes = append(colorBytes, m.Pix[off], m.Pix[off+1], m.Pix[off+2])
					alpha = append(alpha, m.Pix[off+3])
					off += 4
				}
			}
			return colorBytes, opaqueMask(alpha)
		case *image.Paletted:
			colorBytes = make([]byte, 0, width*height*3)
			for y := 0; y < height; y++ {
				off := m.PixOffset(b.Min.X, b.Min.Y+y)
				for x := 0; x < width; x++ {
					c := m.Palette[m.Pix[off]]
					r, g, bb, a := c.RGBA()
					colorBytes = append(colorBytes, byte(r>>8), byte(g>>8), byte(bb>>8))
					alpha = append(alpha, byte(a>>8))
					off++
				}
			}
			return colorBytes, opaqueMask(alpha)
		}
	}
	if bits == 16 {
		if m, ok := img.(*image.NRGBA64); ok {
			// Non-premultiplied 16-bit RGBA: six colour bytes, then the alpha.
			colorBytes = make([]byte, 0, width*height*6)
			for y := 0; y < height; y++ {
				off := m.PixOffset(b.Min.X, b.Min.Y+y)
				for x := 0; x < width; x++ {
					colorBytes = append(colorBytes, m.Pix[off], m.Pix[off+1], m.Pix[off+2], m.Pix[off+3], m.Pix[off+4], m.Pix[off+5])
					alpha = append(alpha, m.Pix[off+6])
					off += 8
				}
			}
			return colorBytes, opaqueMask(alpha)
		}
	}
	if bits == 16 {
		colorBytes = make([]byte, 0, width*height*6)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := color.NRGBA64Model.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA64)
				for _, v := range []uint16{c.R, c.G, c.B} {
					colorBytes = append(colorBytes, byte(v>>8), byte(v))
				}
				alpha = append(alpha, byte(c.A>>8))
			}
		}
		return colorBytes, opaqueMask(alpha)
	}
	colorBytes = make([]byte, 0, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			colorBytes = append(colorBytes, c.R, c.G, c.B)
			alpha = append(alpha, c.A)
		}
	}
	return colorBytes, opaqueMask(alpha)
}

// alphaOf returns the alpha of one pixel as an 8-bit value.
func alphaOf(img image.Image, x, y int) uint8 {
	switch m := img.(type) {
	case *image.NRGBA:
		return m.NRGBAAt(x, y).A
	case *image.RGBA:
		return m.RGBAAt(x, y).A
	case *image.NRGBA64:
		return uint8(m.NRGBA64At(x, y).A >> 8)
	case *image.RGBA64:
		return uint8(m.RGBA64At(x, y).A >> 8)
	case *image.Gray:
		return 0xFF
	case *image.Gray16:
		return 0xFF
	case *image.Paletted:
		_, _, _, a := m.At(x, y).RGBA()
		return uint8(a >> 8)
	default:
		_, _, _, a := img.At(x, y).RGBA()
		return uint8(a >> 8)
	}
}

// opaqueMask drops the alpha channel when every pixel is opaque, so a PNG
// without transparency does not pay for a soft mask.
func opaqueMask(alpha []byte) []byte {
	for _, a := range alpha {
		if a != 0xFF {
			return alpha
		}
	}
	return nil
}

// decodedPNG decodes a PNG once and returns both the image and its header. The
// decoded image is reused by the writer when the caller already has it.
func decodedPNG(data []byte, guard *budget.Guard) (image.Image, pngHeader, error) {
	header, err := readPNGHeader(data)
	if err != nil {
		return nil, pngHeader{}, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, pngHeader{}, fmt.Errorf("pdf: decoding png: %w", err)
	}
	return img, header, nil
}
