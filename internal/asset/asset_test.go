package asset_test

import (
	"testing"

	"github.com/vyachin/go-pdf/internal/asset"
)

func TestResolveLocalPaths(t *testing.T) {
	for _, tc := range []struct{ src, path string }{
		{"logo.png", "logo.png"},
		{" \t\r\n\flogo.png\f\n\r\t ", "logo.png"},
		{"images/logo.png?v=2#preview", "images/logo.png"},
		{"logo.png#preview?v=2", "logo.png"},
		{"images/My%20logo%2B1.png", "images/My logo+1.png"},
		{"images/logo+1.png", "images/logo+1.png"},
		{"%D0%BB%D0%BE%D0%B3%D0%BE.png", "лого.png"},
		{"logo%3Fv%3D2%23small.png?ignored", "logo?v=2#small.png"},
		{"100%.png", "100%.png"},
		{"logo%2G.png", "logo%2G.png"},
		{"logo%2520.png", "logo%20.png"},
		{"../images/logo.png", "../images/logo.png"},
		{"/images/logo.png", "/images/logo.png"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, err := asset.Resolve(tc.src)
			want := asset.Ref{Path: tc.path}
			if err != nil || got != want {
				t.Fatalf("Resolve(%q) = %+v, %v; want %+v, nil", tc.src, got, err, want)
			}
		})
	}
}

func TestResolveRemoteAndDataReferences(t *testing.T) {
	for _, src := range []string{"http://example.test/a.png", "HTTPS://example.test/a.png?q#f", "ftp://example.test/a.png", "//example.test/a.png"} {
		t.Run(src, func(t *testing.T) {
			got, err := asset.Resolve(src)
			if err != nil || got != (asset.Ref{Remote: true}) {
				t.Fatalf("Resolve = %+v, %v; want a remote reference without a local path", got, err)
			}
		})
	}
	for _, src := range []string{"data:image/png;base64,AA==", " \tDATA:image/jpeg;base64,AA==\n"} {
		got, err := asset.Resolve(src)
		if err != nil || got != (asset.Ref{Data: true}) {
			t.Errorf("Resolve(%q) = %+v, %v; want a data reference", src, got, err)
		}
	}
}

func TestResolveRejectsReferencesWithoutAFile(t *testing.T) {
	for _, src := range []string{"", " \t\r\n\f", "?version=1", "#anchor", " \t?# "} {
		got, err := asset.Resolve(src)
		if err == nil || got != (asset.Ref{}) {
			t.Errorf("Resolve(%q) = %+v, %v; want an error and no reference", src, got, err)
		}
	}
}

func TestImageMagicAndDiagnosticNames(t *testing.T) {
	for _, tc := range []struct{ data, format, name string }{
		{"\x89PNG\r\n\x1a\n", "png", "PNG"},
		{"\xff\xd8\xff\xe0", "jpeg", "JPEG"},
		{"GIF87a", "", "GIF"},
		{"GIF89a", "", "GIF"},
		{"BM", "", "BMP"},
		{"RIFF", "", "WebP or another RIFF format"},
		{"\x00\x00\x01\x00", "", "ICO"},
		{"<?xml", "", "SVG"},
		{"%PDF-1.7", "", "PDF"},
		{"", "", "an unrecognised format"},
		{"logo.png", "", "an unrecognised format"},
		{"\xff\xd8", "", "an unrecognised format"},
		{"\x89PNG\r\n\x1a", "", "an unrecognised format"},
		{"GIF89", "", "an unrecognised format"},
	} {
		t.Run(tc.name+"/"+tc.data, func(t *testing.T) {
			data := []byte(tc.data)
			if got := asset.Sniff(data); got != tc.format {
				t.Errorf("Sniff = %q, want %q", got, tc.format)
			}
			if got := asset.FormatName(data); got != tc.name {
				t.Errorf("FormatName = %q, want %q", got, tc.name)
			}
		})
	}
}
