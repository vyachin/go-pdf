package engine

import (
	"bytes"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/profile"
)

// imageLoader resolves <img src> to decoded image data. Decoded images are
// cached for the conversion so that a repeated source is read and embedded once.
type imageLoader struct {
	baseDir string
	mu      sync.Mutex
	cache   map[string]*layout.ImageData
	// loads counts actual decodes, used by tests to prove caching.
	loads int
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
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.baseDir, path)
	}
	data, err := profile.ReadWithin(l.baseDir, path)
	if err != nil {
		if errors.Is(err, profile.ErrOutsideRoot) {
			return nil, fmt.Errorf("image %q is outside the base directory", src)
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
