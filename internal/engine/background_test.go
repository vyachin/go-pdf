package engine

import (
	"bytes"
	"context"
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
