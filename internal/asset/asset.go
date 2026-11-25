// Package asset resolves a document's asset references — images, stylesheets
// and fonts — the way a browser resolves a URL path, so a template copied from
// the web names the local file it means. One resolver serves every asset kind:
// a new scheme or reference form is added here, not in each loader.
package asset

import (
	"fmt"
	"net/url"
	"strings"
)

// Ref is one resolved asset reference.
type Ref struct {
	// Path is the local path the reference names. It is empty for a remote
	// reference.
	Path string
	// Remote reports a reference with a scheme or a scheme-relative host.
	Remote bool
	// Data reports a data: URI, whose payload the caller decodes itself.
	Data bool
}

// asciiSpace is the whitespace HTML trims around an attribute value.
const asciiSpace = " \t\r\n\f"

// Resolve parses src as a URL path. ASCII whitespace is trimmed, the query and
// the fragment are removed, the path is percent-decoded, and a scheme, a
// scheme-relative //host and a data: URI are classified instead of being read
// as file names. A percent sign that is not a valid escape is kept verbatim,
// because such a name is a legal file name.
func Resolve(src string) (Ref, error) {
	s := strings.Trim(src, asciiSpace)
	if s == "" {
		return Ref{}, fmt.Errorf("empty asset reference")
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "data:"):
		return Ref{Data: true}, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(s, "//"), strings.HasPrefix(lower, "ftp://"):
		return Ref{Remote: true}, nil
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return Ref{}, fmt.Errorf("asset reference %q names no file", strings.Trim(src, asciiSpace))
	}
	path, err := url.PathUnescape(s)
	if err != nil {
		path = s
	}
	if path == "" {
		return Ref{}, fmt.Errorf("asset reference %q names no file", strings.Trim(src, asciiSpace))
	}
	return Ref{Path: path}, nil
}

// Sniff returns the image format of data from its magic bytes: "png" or
// "jpeg". The format comes from the content, never from the extension, so a PNG
// named .jpg still loads as a PNG.
func Sniff(data []byte) string {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpeg"
	}
	return ""
}

// FormatName names the format of data for a diagnostic: the detected image
// format when it is one, otherwise a short description of the magic bytes.
func FormatName(data []byte) string {
	switch Sniff(data) {
	case "png":
		return "PNG"
	case "jpeg":
		return "JPEG"
	}
	switch {
	case len(data) >= 6 && string(data[:6]) == "GIF87a", len(data) >= 6 && string(data[:6]) == "GIF89a":
		return "GIF"
	case len(data) >= 2 && string(data[:2]) == "BM":
		return "BMP"
	case len(data) >= 4 && string(data[:4]) == "RIFF":
		return "WebP or another RIFF format"
	case len(data) >= 4 && string(data[:4]) == "\x00\x00\x01\x00":
		return "ICO"
	case len(data) >= 5 && string(data[:5]) == "<?xml":
		return "SVG"
	case len(data) >= 4 && (string(data[:4]) == "%PDF"):
		return "PDF"
	}
	return "an unrecognised format"
}
