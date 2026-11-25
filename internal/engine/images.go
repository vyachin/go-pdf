package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/layout"
)

// imageLoader resolves <img src> to decoded image data. Decoded images are
// cached for the conversion so that a repeated source is read and embedded once.
type imageLoader struct {
	baseDir   string
	maxPixels int
	maxBytes  int
	// Aggregate caps for one conversion: encoded bytes and decoded pixels of
	// every image, counted once per identity.
	maxTotalBytes  int
	maxTotalPixels int
	totalBytes     int64
	totalPixels    int64
	// guard bounds decoding work and carries cancellation. Nil means no limit.
	guard     *budget.Guard
	root      *fsx.Root
	rootErr   error
	rootOpens int // resolved roots, used by tests to prove reuse
	mu        sync.Mutex
	cache     map[string]*layout.ImageData
	// loads counts actual decodes, used by tests to prove caching.
	loads int
}

// Close releases the lazily opened root.
func (l *imageLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.root == nil {
		return nil
	}
	err := l.root.Close()
	l.root = nil
	return err
}

// Load implements layout.ImageLoader. A file is identified by its canonical
// absolute path after symlinks, so every spelling of one file shares one cache
// entry and one PDF object. A data: URI is identified by the SHA-256 of its
// decoded bytes, so a repeated URI is decoded once.
func (l *imageLoader) Load(src string) (*layout.ImageData, error) {
	if src == "" {
		return nil, fmt.Errorf("image has no src")
	}
	lower := strings.ToLower(src)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("remote images are not supported: %s", src)
	}
	// A data: URI needs no file, so it must not open the base directory (which
	// may be unset or absent) or count as a root read.
	if strings.HasPrefix(lower, "data:") {
		return l.loadDataURI(src)
	}
	l.mu.Lock()
	if l.root == nil && l.rootErr == nil {
		l.rootOpens++
		l.root, l.rootErr = fsx.Open(l.baseDir)
	}
	root := l.root
	rootErr := l.rootErr
	l.mu.Unlock()
	if rootErr != nil {
		return nil, fmt.Errorf("opening the base directory: %w", rootErr)
	}
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.baseDir, path)
	}
	canonical, err := root.Resolve(path)
	if err != nil {
		if errors.Is(err, fsx.ErrOutsideRoot) {
			return nil, fmt.Errorf("image %q is outside the base directory", src)
		}
		return nil, err
	}
	key := "file:" + canonical
	l.mu.Lock()
	if img, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return img, nil
	}
	l.mu.Unlock()
	// The size is known before the file is opened, so an image past the per-file
	// or aggregate limit is never read into memory.
	size, err := root.Size(canonical)
	if err != nil {
		return nil, err
	}
	if l.maxBytes > 0 && size > int64(l.maxBytes) {
		return nil, fmt.Errorf("%w: image %q exceeds the limit of %d bytes", budget.ErrExceeded, src, l.maxBytes)
	}
	if err := l.admitBytes(size); err != nil {
		return nil, err
	}
	data, err := root.ReadLimit(canonical, l.maxBytes)
	if err != nil {
		if errors.Is(err, fsx.ErrOutsideRoot) {
			return nil, fmt.Errorf("image %q is outside the base directory", src)
		}
		if errors.Is(err, fsx.ErrTooLarge) {
			return nil, fmt.Errorf("%w: image %q exceeds the limit of %d bytes", budget.ErrExceeded, src, l.maxBytes)
		}
		return nil, err
	}
	switch {
	case strings.HasSuffix(lower, ".png"):
		return l.decodePNG(src, path, data, key)
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return l.decodeJPEG(src, path, data, key)
	default:
		return nil, fmt.Errorf("unsupported image format: %s (only png and jpeg are supported)", src)
	}
}

// admitBytes counts one image's encoded size against the conversion total. It
// reports resource/limit before the file is read.
func (l *imageLoader) admitBytes(n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxTotalBytes > 0 && l.totalBytes+n > int64(l.maxTotalBytes) {
		return fmt.Errorf("%w: the document's images exceed %d encoded bytes in total", budget.ErrExceeded, l.maxTotalBytes)
	}
	l.totalBytes += n
	return nil
}

// admitPixels counts one image's pixels against the conversion total. It is
// called after the header check and before the pixels are decoded.
func (l *imageLoader) admitPixels(n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxTotalPixels > 0 && l.totalPixels+n > int64(l.maxTotalPixels) {
		return fmt.Errorf("%w: the document's images exceed %d pixels in total", budget.ErrExceeded, l.maxTotalPixels)
	}
	l.totalPixels += n
	return nil
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
	sum := sha256.Sum256(data)
	key := "sha256:" + hex.EncodeToString(sum[:])
	l.mu.Lock()
	if img, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return img, nil
	}
	l.mu.Unlock()
	if l.maxBytes > 0 && len(data) > l.maxBytes {
		return nil, fmt.Errorf("%w: data: URI image exceeds the limit of %d bytes", budget.ErrExceeded, l.maxBytes)
	}
	// The encoded bytes are already in the document, but the size is counted
	// before the pixels are decoded.
	if err := l.admitBytes(int64(len(data))); err != nil {
		return nil, err
	}
	switch mime {
	case "image/png":
		return l.decodePNG(src, "data:image/png", data, key)
	case "image/jpeg", "image/jpg":
		return l.decodeJPEG(src, "data:image/jpeg", data, key)
	default:
		return nil, fmt.Errorf("unsupported data: URI image type %q (only png and jpeg are supported)", mime)
	}
}

// decodePNG builds image data from PNG bytes and checks the pixel budgets.
func (l *imageLoader) decodePNG(src, name string, data []byte, key string) (*layout.ImageData, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if l.maxPixels > 0 && cfg.Width*cfg.Height > l.maxPixels {
		return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, cfg.Width*cfg.Height, l.maxPixels)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if err := l.admitPixels(pixels); err != nil {
		return nil, err
	}
	// Charge the work before decoding: the header already tells how many pixels
	// the decoder will allocate.
	if err := l.charge(pixels); err != nil {
		return nil, err
	}
	l.countDecode()
	// The writer decodes the pixels when it embeds the image, so a truncated
	// file passes the header check and fails only at render time. Decoding
	// here keeps Validate and Convert reporting the same problem.
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return l.store(key, &layout.ImageData{
		Width:  int32(cfg.Width) * 750,
		Height: int32(cfg.Height) * 750,
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "png",
		Data:   data,
	}), nil
}

// decodeJPEG builds image data from JPEG bytes without recompression.
func (l *imageLoader) decodeJPEG(src, name string, data []byte, key string) (*layout.ImageData, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if cfg.ColorModel == color.CMYKModel {
		// The writer has no colour space for CMYK, and Validate must find
		// this at image load time just like Convert does.
		return nil, fmt.Errorf("pdf: CMYK JPEG images are not supported")
	}
	if l.maxPixels > 0 && cfg.Width*cfg.Height > l.maxPixels {
		return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, cfg.Width*cfg.Height, l.maxPixels)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if err := l.admitPixels(pixels); err != nil {
		return nil, err
	}
	// A JPEG is embedded without decoding, so only the image itself is
	// charged here.
	if err := l.charge(0); err != nil {
		return nil, err
	}
	l.countDecode()
	// A JPEG is embedded as DCTDecode without decoding, so the header check
	// is the whole check Convert performs.
	return l.store(key, &layout.ImageData{
		Width:  int32(cfg.Width) * 750,
		Height: int32(cfg.Height) * 750,
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "jpeg",
		Data:   data,
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
