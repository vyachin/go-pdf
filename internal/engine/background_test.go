package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBackgroundPNG writes a small solid PNG the background tests can embed.
func writeBackgroundPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 60, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 60; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 150, B: 100, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundImageIsEmbedded(t *testing.T) {
	dir := t.TempDir()
	writeBackgroundPNG(t, filepath.Join(dir, "cover.png"))
	src := `<div style="width:100pt;height:50pt;background-image:url(cover.png)">x</div>`
	res, err := Convert(context.Background(), []byte(src), Options{
		File:         "bg.html",
		BaseDir:      dir,
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Contains(res.PDF, []byte("/Subtype /Image")) {
		t.Fatal("the background image was not embedded in the PDF")
	}
}

func TestBackgroundImageErrors(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "doc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeBackgroundPNG(t, filepath.Join(root, "outside.png"))

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "missing file",
			src:  `<div style="background-image:url(missing.png)">x</div>`,
			want: "missing.png",
		},
		{
			name: "outside the base directory",
			src:  `<div style="background-image:url(../outside.png)">x</div>`,
			want: "outside.png",
		},
		{
			name: "remote source",
			src:  `<div style="background-image:url(https://example.com/cover.jpg)">x</div>`,
			want: "remote background images",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				File:         "bg.html",
				BaseDir:      dir,
				FallbackFont: testFontPath(t),
			})
			if err == nil {
				t.Fatalf("%s: conversion succeeded", tc.name)
			}
			msg := err.Error() + "\n" + strings.Join(res.Diagnostics.Lines(), "\n")
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("%s: %q does not name %q", tc.name, msg, tc.want)
			}
		})
	}
}

// dataURIPNG builds a small in-memory PNG and returns it as a data: URI.
func dataURIPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 6, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 90, 140, 190, 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDataURIImageIsEmbedded(t *testing.T) {
	uri := dataURIPNG(t)
	src := `<div style="width:60pt;height:30pt;background-image:url('` + uri + `')">bg</div>` +
		`<p><img src="` + uri + `"></p>`
	res, err := Convert(context.Background(), []byte(src), Options{
		File:         "data-uri.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Contains(res.PDF, []byte("/Subtype /Image")) {
		t.Fatal("the data: URI image was not embedded in the PDF")
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the same data: URI was embedded %d times, want 1", n)
	}
	again, err := Convert(context.Background(), []byte(src), Options{
		File:         "data-uri.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.PDF, again.PDF) {
		t.Fatal("two conversions of the same data: URI document differ")
	}
}

func TestDataURIRejectsUnsupportedPayloads(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"svg", `<img src="data:image/svg+xml;base64,PHN2Zy8+">`},
		{"not-base64", `<img src="data:image/png,raw">`},
		{"broken-base64", `<img src="data:image/png;base64,@@@@">`},
		{"not-an-image", `<img src="data:text/plain;base64,aGk=">`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				File:         tc.name + ".html",
				FallbackFont: testFontPath(t),
			})
			if err == nil {
				t.Fatal("expected a diagnostic for the unsupported data: URI")
			}
			if len(res.Diagnostics) == 0 {
				t.Fatal("rejected without diagnostics")
			}
		})
	}
}
