package engine

import (
	"bytes"
	"encoding/base64"
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

// Load implements layout.ImageLoader.
func (l *imageLoader) Load(src string) (*layout.ImageData, error) {
	if src == "" {
		return nil, fmt.Errorf("image has no src")
	}
	l.mu.Lock()
	if img, ok := l.cache[src]; ok {
		l.mu.Unlock()
		return img, nil
	}
	l.mu.Unlock()
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
	data, err := root.ReadLimit(path, l.maxBytes)
	if err != nil {
		if errors.Is(err, fsx.ErrOutsideRoot) {
			return nil, fmt.Errorf("image %q is outside the base directory", src)
		}
		if errors.Is(err, fsx.ErrTooLarge) {
			return nil, fmt.Errorf("%w: image %q exceeds the limit of %d bytes", budget.ErrExceeded, src, l.maxBytes)
		}
		return nil, err
	}
	l.mu.Lock()
	l.loads++
	l.mu.Unlock()
	switch {
	case strings.HasSuffix(lower, ".png"):
		return l.decodePNG(src, path, data)
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return l.decodeJPEG(src, path, data)
	default:
		return nil, fmt.Errorf("unsupported image format: %s (only png and jpeg are supported)", src)
	}
}

// loadDataURI decodes an image embedded in the document as a data: URI. Only
// base64-encoded PNG and JPEG are supported, matching the file loader.
func (l *imageLoader) loadDataURI(src string) (*layout.ImageData, error) {
	comma := strings.IndexByte(src, ',')
	if comma < 0 {
		return nil, fmt.Errorf("malformed data: URI image: missing ','")
	}
	header := strings.ToLower(strings.TrimSpace(src[len("data:"):comma]))
	payload := src[comma+1:]
	if !strings.HasSuffix(header, ";base64") {
		return nil, fmt.Errorf("data: URI images must be base64-encoded")
	}
	mime := strings.TrimSuffix(header, ";base64")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
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
	if l.maxBytes > 0 && len(data) > l.maxBytes {
		return nil, fmt.Errorf("%w: data: URI image exceeds the limit of %d bytes", budget.ErrExceeded, l.maxBytes)
	}
	switch mime {
	case "image/png":
		return l.decodePNG(src, "data:image/png", data)
	case "image/jpeg", "image/jpg":
		return l.decodeJPEG(src, "data:image/jpeg", data)
	default:
		return nil, fmt.Errorf("unsupported data: URI image type %q (only png and jpeg are supported)", mime)
	}
}

// decodePNG builds image data from PNG bytes and checks the pixel budget.
func (l *imageLoader) decodePNG(src, name string, data []byte) (*layout.ImageData, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if l.maxPixels > 0 && cfg.Width*cfg.Height > l.maxPixels {
		return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, cfg.Width*cfg.Height, l.maxPixels)
	}
	// The writer decodes the pixels when it embeds the image, so a truncated
	// file passes the header check and fails only at render time. Decoding
	// here keeps Validate and Convert reporting the same problem.
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return l.store(src, &layout.ImageData{
		Width:  int32(cfg.Width) * 750,
		Height: int32(cfg.Height) * 750,
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "png",
		Data:   data,
	}), nil
}

// decodeJPEG builds image data from JPEG bytes without recompression.
func (l *imageLoader) decodeJPEG(src, name string, data []byte) (*layout.ImageData, error) {
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
	// A JPEG is embedded as DCTDecode without decoding, so the header check
	// is the whole check Convert performs.
	return l.store(src, &layout.ImageData{
		Width:  int32(cfg.Width) * 750,
		Height: int32(cfg.Height) * 750,
		PixelW: cfg.Width,
		PixelH: cfg.Height,
		Format: "jpeg",
		Data:   data,
	}), nil
}

func (l *imageLoader) store(src string, img *layout.ImageData) *layout.ImageData {
	l.mu.Lock()
	if l.cache == nil {
		l.cache = map[string]*layout.ImageData{}
	}
	l.cache[src] = img
	l.mu.Unlock()
	return img
}
