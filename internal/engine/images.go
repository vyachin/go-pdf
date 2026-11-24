package engine

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/vyachin/go-pdf/internal/layout"
)

// imageLoader resolves <img src> to decoded image data.
type imageLoader struct {
	baseDir string
}

// Load implements layout.ImageLoader.
func (l *imageLoader) Load(src string) (*layout.ImageData, error) {
	if src == "" {
		return nil, fmt.Errorf("image has no src")
	}
	lower := strings.ToLower(src)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("remote images are not supported: %s", src)
	}
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.baseDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasSuffix(lower, ".png"):
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return &layout.ImageData{
			Width:  int32(cfg.Width) * 750,
			Height: int32(cfg.Height) * 750,
			PixelW: cfg.Width,
			PixelH: cfg.Height,
			Format: "png",
			Data:   data,
		}, nil
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return &layout.ImageData{
			Width:  int32(cfg.Width) * 750,
			Height: int32(cfg.Height) * 750,
			PixelW: cfg.Width,
			PixelH: cfg.Height,
			Format: "jpeg",
			Data:   data,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported image format: %s (only png and jpeg are supported)", src)
	}
}
