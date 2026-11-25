package engine

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestIdenticalInputsReproduceAndMutationChangesThem guards the boundary the
// documentation states: an explicit bundled font and immutable assets reproduce
// one PDF, while replacing an image or a stylesheet under the same path is a
// changed input. Nothing is cached across conversions, so the next conversion
// reads the new bytes.
func TestIdenticalInputsReproduceAndMutationChangesThem(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "logo.png"))
	writeFile(t, filepath.Join(dir, "style.css"), []byte("p { color: #112233 }"))
	src := []byte(`<link rel="stylesheet" href="style.css"><p>Текст</p><img src="logo.png">`)
	opts := Options{BaseDir: dir, FallbackFont: testFontPath(t), File: "reproducible.html"}
	convert := func() []byte {
		t.Helper()
		res, err := Convert(context.Background(), src, opts)
		if err != nil {
			t.Fatal(err)
		}
		return res.PDF
	}

	first := convert()
	if second := convert(); !bytes.Equal(first, second) {
		t.Fatal("identical complete inputs produced different bytes")
	}

	// The same path now holds a different image: the complete input changed.
	var mutated bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 7, 7))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 20, 200, 90, 255
	}
	if err := png.Encode(&mutated, img); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "logo.png"), mutated.Bytes())
	withNewImage := convert()
	if bytes.Equal(first, withNewImage) {
		t.Fatal("a replaced image did not change the output")
	}

	// The same path now holds a different stylesheet.
	writeFile(t, filepath.Join(dir, "style.css"), []byte("p { color: #ff0000 }"))
	withNewSheet := convert()
	if bytes.Equal(withNewImage, withNewSheet) {
		t.Fatal("a replaced stylesheet did not change the output")
	}
	if bytes.Equal(first, withNewSheet) {
		t.Fatal("two changed assets produced the original bytes")
	}
}

// writeFile writes data to path, failing the test on error.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
