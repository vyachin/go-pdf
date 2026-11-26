package engine

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"math"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/asset"
	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/layout"
)

// imageLoader resolves <img src> to decoded image data. Decoded images are
// cached for the conversion so that a repeated source is read and embedded once.
type imageLoader struct {
	baseDir string
	// source replaces the host filesystem for every read when it is set.
	source    fs.FS
	maxPixels int
	maxBytes  int
	// Aggregate caps for one conversion: encoded bytes and decoded pixels of
	// every image, counted once per identity.
	maxTotalBytes  int
	maxTotalPixels int
	// guard bounds decoding work and carries cancellation. Nil means no limit.
	guard *budget.Guard
	// reg is the conversion's asset registry, shared with the font and
	// stylesheet adapters. A loader built without one — a test, or a standalone
	// callback — creates and owns its own registry.
	reg         *assets.Registry
	ownRegistry bool
	mu          sync.Mutex
	cache       map[string]*layout.ImageData
	// loads counts actual decodes, used by tests to prove caching.
	loads int
}

// assets returns the conversion's registry, creating one when the loader was
// built without it.
func (l *imageLoader) assets() *assets.Registry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reg == nil {
		l.reg = assets.NewWithSource(l.guard, l.source)
		l.ownRegistry = true
	}
	return l.reg
}

// Close releases a registry the loader created itself. A registry owned by the
// conversion is closed by the conversion, so this is a no-op for it.
func (l *imageLoader) Close() error {
	l.mu.Lock()
	reg, own := l.reg, l.ownRegistry
	l.reg, l.ownRegistry = nil, false
	l.mu.Unlock()
	if own && reg != nil {
		return reg.Close()
	}
	return nil
}

// Load implements layout.ImageLoader. A file is identified by its canonical
// absolute path after symlinks, so every spelling of one file shares one cache
// entry and one PDF object. A data: URI is identified by the SHA-256 of its
// decoded bytes, so a repeated URI is decoded once.
func (l *imageLoader) Load(src string) (*layout.ImageData, error) {
	if src == "" {
		return nil, fmt.Errorf("image has no src")
	}
	ref, err := asset.Resolve(src)
	if err != nil {
		return nil, err
	}
	if ref.Remote {
		return nil, fmt.Errorf("remote images are not supported: %s", src)
	}
	// A data: URI needs no file, so it must not open the base directory (which
	// may be unset or absent) or count as a root read.
	if ref.Data {
		return l.loadDataURI(src)
	}
	rec, err := l.assets().Read(assets.Request{
		Kind:          assets.Image,
		Reference:     src,
		Dir:           l.baseDir,
		MaxBytes:      l.maxBytes,
		MaxTotalBytes: int64(l.maxTotalBytes),
	})
	if err != nil {
		return nil, l.readError(err, src)
	}
	l.mu.Lock()
	if img, ok := l.cache[rec.Key]; ok {
		l.mu.Unlock()
		return img, nil
	}
	l.mu.Unlock()
	// The format comes from the content, not from the extension: a PNG named
	// .jpg or without an extension still loads, and a genuinely unsupported
	// file is named by what it is.
	switch asset.Sniff(rec.Bytes) {
	case "png":
		return l.decodePNG(src, rec.Path, rec)
	case "jpeg":
		return l.decodeJPEG(src, rec.Path, rec)
	default:
		l.assets().Discard(rec)
		return nil, fmt.Errorf("unsupported image format %s in %q (only PNG and JPEG are supported)", asset.FormatName(rec.Bytes), src)
	}
}

// readError maps one registry failure to the diagnostic the image loader
// reports, keeping the reference the document wrote.
func (l *imageLoader) readError(err error, src string) error {
	var assetErr *assets.Error
	if errors.As(err, &assetErr) {
		switch {
		case errors.Is(assetErr.Err, fsx.ErrOutsideRoot):
			return fmt.Errorf("image %q is outside the base directory", src)
		case errors.Is(assetErr.Err, assets.ErrTooLarge):
			return fmt.Errorf("%w: image %q exceeds the limit of %d bytes", budget.ErrExceeded, src, l.maxBytes)
		}
		if assetErr.Path != "" {
			// The message names the file the reference resolved to, so a query
			// string or a fragment never reaches the diagnostic.
			return fmt.Errorf("image %q: %w", assetErr.Path, assetErr.Err)
		}
	}
	return err
}

// intrinsicMillipoints converts an intrinsic pixel count to millipoints at the
// 96 dpi CSS ratio. The product is computed in 64-bit and clamped: a legal very
// wide image would otherwise wrap to a negative width and reach the PDF as a
// negative image scale.
func intrinsicMillipoints(px int) int32 {
	v := int64(px) * 750
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v)
}

func (l *imageLoader) countDecode() {
	l.mu.Lock()
	l.loads++
	l.mu.Unlock()
}

// charge records the work of one image: one unit for the image itself and one
// per 64 KiB of pixels it decodes. A cancelled context is reported before the
// decode starts. A nil guard means unlimited work.
func (l *imageLoader) charge(pixels int64) error {
	if l.guard == nil {
		return nil
	}
	if err := l.guard.Err(); err != nil {
		return err
	}
	units := uint64(1)
	if pixels > 0 {
		units += uint64(pixels) / 65536
	}
	return l.guard.Add(units)
}

// loadDataURI decodes an image embedded in the document as a data: URI. Only
// base64-encoded PNG and JPEG are supported, matching the file loader.
func (l *imageLoader) loadDataURI(src string) (*layout.ImageData, error) {
	head, payload, ok := strings.Cut(src, ",")
	if !ok {
		return nil, fmt.Errorf("malformed data: URI image: missing ','")
	}
	// Load matched the "data:" scheme case-insensitively before calling.
	header := strings.ToLower(strings.TrimSpace(head[len("data:"):]))
	media, ok := strings.CutSuffix(header, ";base64")
	if !ok {
		return nil, fmt.Errorf("data: URI images must be base64-encoded")
	}
	// A parameter list follows the media type; the type ends at the first
	// semicolon.
	mime, _, _ := strings.Cut(media, ";")
	payload = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			return -1
		}
		return r
	}, payload)
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("data: URI image: %v", err)
	}
	rec, err := l.assets().AdmitData(assets.Image, src, data, l.maxBytes, int64(l.maxTotalBytes))
	if err != nil {
		return nil, l.readError(err, src)
	}
	l.mu.Lock()
	if img, ok := l.cache[rec.Key]; ok {
		l.mu.Unlock()
		return img, nil
	}
	l.mu.Unlock()
	name := "data:" + mime
	switch asset.Sniff(data) {
	case "png":
		return l.decodePNG(src, name, rec)
	case "jpeg":
		return l.decodeJPEG(src, name, rec)
	default:
		l.assets().Discard(rec)
		return nil, fmt.Errorf("unsupported data: URI image type %q: the payload is %s (only PNG and JPEG are supported)", mime, asset.FormatName(data))
	}
}

// decodePNG builds image data from PNG bytes and checks the pixel budgets.
func (l *imageLoader) decodePNG(src, name string, rec *assets.Record) (*layout.ImageData, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(rec.Bytes))
	if err != nil {
		l.assets().Discard(rec)
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	// The area is multiplied in 64 bits before it is compared: on a 32-bit
	// build the product of two 16-bit dimensions wraps and would bypass the
	// limit.
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if l.maxPixels > 0 && pixels > int64(l.maxPixels) {
		l.assets().Discard(rec)
		return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, pixels, l.maxPixels)
	}
	if err := l.assets().AdmitPixels(pixels, int64(l.maxTotalPixels)); err != nil {
		l.assets().Discard(rec)
		return nil, err
	}
	// Charge the work before decoding: the header already tells how many pixels
	// the decoder will allocate.
	if err := l.charge(pixels); err != nil {
		l.assets().Discard(rec)
		l.assets().RefundPixels(pixels)
		return nil, err
	}
	l.countDecode()
	// The writer embeds the pixels, so a truncated file must fail here: decoding
	// keeps Validate and Convert reporting the same problem. The decoded image is
	// kept for the writer, which must not decode it a second time.
	decoded, err := png.Decode(bytes.NewReader(rec.Bytes))
	if err != nil {
		// The image did not decode, so it must not consume the aggregate totals
		// a later valid image needs.
		l.assets().Discard(rec)
		l.assets().RefundPixels(pixels)
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return l.store(rec.Key, &layout.ImageData{
		Width:   intrinsicMillipoints(cfg.Width),
		Height:  intrinsicMillipoints(cfg.Height),
		PixelW:  cfg.Width,
		PixelH:  cfg.Height,
		Format:  "png",
		Data:    rec.Bytes,
		Decoded: decoded,
	}), nil
}

// decodeJPEG builds image data from JPEG bytes without recompression.
func (l *imageLoader) decodeJPEG(src, name string, rec *assets.Record) (*layout.ImageData, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(rec.Bytes))
	if err != nil {
		l.assets().Discard(rec)
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if cfg.ColorModel == color.CMYKModel {
		// The writer has no colour space for CMYK, and Validate must find
		// this at image load time just like Convert does.
		l.assets().Discard(rec)
		return nil, fmt.Errorf("pdf: CMYK JPEG images are not supported")
	}
	// The area is multiplied in 64 bits, like the PNG path: a 32-bit build
	// must not wrap it.
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if l.maxPixels > 0 && pixels > int64(l.maxPixels) {
		l.assets().Discard(rec)
		return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, pixels, l.maxPixels)
	}
	if err := l.assets().AdmitPixels(pixels, int64(l.maxTotalPixels)); err != nil {
		l.assets().Discard(rec)
		return nil, err
	}
	// A JPEG is embedded without decoding, so only the image itself is
	// charged here.
	if err := l.charge(0); err != nil {
		l.assets().Discard(rec)
		l.assets().RefundPixels(pixels)
		return nil, err
	}
	l.countDecode()
	// The data is embedded as DCTDecode without decoding, so it is checked for
	// completeness here: a file cut before its end-of-image marker must fail at
	// load time in both entry points, exactly as a truncated PNG does.
	if !jpegComplete(rec.Bytes) {
		l.assets().Discard(rec)
		l.assets().RefundPixels(pixels)
		return nil, fmt.Errorf("%s: the JPEG ends before its end-of-image marker", name)
	}
	return l.store(rec.Key, &layout.ImageData{
		Width:  intrinsicMillipoints(cfg.Width),
		Height: intrinsicMillipoints(cfg.Height),
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "jpeg",
		Data:   rec.Bytes,
	}), nil
}

func (l *imageLoader) store(key string, img *layout.ImageData) *layout.ImageData {
	l.mu.Lock()
	if l.cache == nil {
		l.cache = map[string]*layout.ImageData{}
	}
	l.cache[key] = img
	l.mu.Unlock()
	return img
}

// jpegComplete reports whether data holds a complete JPEG: it walks the marker
// segments from the start-of-image to the end-of-image marker, skipping the
// entropy-coded data that follows each start-of-scan. A file cut short has no
// end-of-image marker and no complete segment header.
func jpegComplete(data []byte) bool {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return false
	}
	i := 2
	for i+1 < len(data) {
		if data[i] != 0xFF {
			return false
		}
		marker := data[i+1]
		i += 2
		switch {
		case marker == 0xD9: // EOI
			return true
		case marker == 0x01, marker >= 0xD0 && marker <= 0xD7: // standalone markers
			continue
		case marker == 0xDA: // SOS: segment header, then entropy-coded data
			if i+1 >= len(data) {
				return false
			}
			length := int(data[i])<<8 | int(data[i+1])
			if length < 2 {
				return false
			}
			i += length
			// Skip the entropy data up to the next real marker.
			for i+1 < len(data) {
				if data[i] != 0xFF {
					i++
					continue
				}
				next := data[i+1]
				if next == 0x00 || next >= 0xD0 && next <= 0xD7 {
					i += 2
					continue
				}
				if next == 0xFF {
					i++
					continue
				}
				break
			}
			if i+1 >= len(data) {
				return false
			}
		default:
			if i+1 >= len(data) {
				return false
			}
			length := int(data[i])<<8 | int(data[i+1])
			if length < 2 {
				return false
			}
			i += length
		}
	}
	return false
}
