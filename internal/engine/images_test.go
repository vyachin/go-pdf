package engine

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// imageData returns the header and base64 payload of a data: URI for a tiny
// valid image of the given media type.
func imageData(t *testing.T, media string) (string, string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var buf bytes.Buffer
	var err error
	switch media {
	case "image/png":
		err = png.Encode(&buf, img)
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, nil)
	default:
		t.Fatalf("no encoder for %q", media)
	}
	if err != nil {
		t.Fatal(err)
	}
	return media + ";base64", base64.StdEncoding.EncodeToString(buf.Bytes())
}

// chunked inserts whitespace between base64 chunks, as a wrapped data: URI in
// a document may contain.
func chunked(s string, n int) string {
	var b strings.Builder
	for len(s) > n {
		b.WriteString(s[:n])
		b.WriteString("\r\n\t ")
		s = s[n:]
	}
	b.WriteString(s)
	return b.String()
}

func TestLoadDataURI(t *testing.T) {
	pngHeader, pngPayload := imageData(t, "image/png")
	jpegHeader, jpegPayload := imageData(t, "image/jpeg")
	uri := func(header, payload string) string { return "data:" + header + "," + payload }
	cases := []struct {
		name string
		src  string
		// format is the decoded image format on success; wantErr is a substring
		// of the error on failure.
		format  string
		wantErr string
	}{
		{"png", uri(pngHeader, pngPayload), "png", ""},
		{"jpeg", uri(jpegHeader, jpegPayload), "jpeg", ""},
		{"jpg alias", uri(strings.Replace(jpegHeader, "image/jpeg", "image/jpg", 1), jpegPayload), "jpeg", ""},
		{"parameters", uri(strings.Replace(pngHeader, ";base64", ";charset=utf-8;base64", 1), pngPayload), "png", ""},
		{"uppercase header", uri(strings.ToUpper(pngHeader), pngPayload), "png", ""},
		{"whitespace", "data: " + pngHeader + " , " + chunked(pngPayload, 16), "png", ""},

		{"no comma", "data:" + pngHeader, "", "missing ','"},
		{"no base64 marker", uri("image/png", pngPayload), "", "must be base64-encoded"},
		{"empty payload", uri(pngHeader, ""), "", "unexpected EOF"},
		{"unknown media type", uri("image/svg+xml;base64", "PHN2Zy8+"), "", "unsupported data: URI image type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := &imageLoader{}
			img, err := loader.loadDataURI(tc.src)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("decoded %q, want an error mentioning %q", tc.src, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if img.Format != tc.format {
				t.Fatalf("format = %q, want %q", img.Format, tc.format)
			}
			if len(img.Data) == 0 {
				t.Fatal("no image data")
			}
		})
	}
}
