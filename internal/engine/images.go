package engine

import (
	"bytes"
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
	lower := strings.ToLower(src)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("remote images are not supported: %s", src)
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
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if l.maxPixels > 0 && cfg.Width*cfg.Height > l.maxPixels {
			return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, cfg.Width*cfg.Height, l.maxPixels)
		}
		return l.store(src, &layout.ImageData{
			Width:  int32(cfg.Width) * 750,
			Height: int32(cfg.Height) * 750,
			PixelW: cfg.Width,
			PixelH: cfg.Height,
			Format: "png",
			Data:   data,
		}), nil
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if cfg.ColorModel == color.CMYKModel {
			// The writer has no colour space for CMYK, and Validate must find
			// this at image load time just like Convert does.
			return nil, fmt.Errorf("pdf: CMYK JPEG images are not supported")
		}
		if l.maxPixels > 0 && cfg.Width*cfg.Height > l.maxPixels {
			return nil, fmt.Errorf("%w: image %q is %d pixels, the limit is %d", budget.ErrExceeded, src, cfg.Width*cfg.Height, l.maxPixels)
		}
		return l.store(src, &layout.ImageData{
			Width:  int32(cfg.Width) * 750,
			Height: int32(cfg.Height) * 750,
			PixelW: cfg.Width,
			PixelH: cfg.Height,
			Format: "jpeg",
			Data:   data,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported image format: %s (only png and jpeg are supported)", src)
	}
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
