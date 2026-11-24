package profile

import (
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
)

// FontSource is a resolved @font-face source.
type FontSource struct {
	Path string
}

// ParseFontFaceSrc parses the value of a src descriptor. Only a single local
// url(...) is supported: remote fonts and format hints are rejected because the
// engine never performs network access.
func ParseFontFaceSrc(toks []css.Token) (FontSource, error) {
	var f []css.Token
	for _, t := range toks {
		if t.Kind == css.WhitespaceToken || t.Kind == css.EOFToken {
			continue
		}
		f = append(f, t)
	}
	if len(f) == 0 {
		return FontSource{}, fmt.Errorf("expected url(...)")
	}
	if len(f) != 1 || f[0].Kind != css.URLToken {
		return FontSource{}, fmt.Errorf("only a single local url(...) source is supported")
	}
	path := trimQuotes(f[0].Text)
	if path == "" {
		return FontSource{}, fmt.Errorf("empty font path")
	}
	if strings.Contains(path, "://") {
		return FontSource{}, fmt.Errorf("remote fonts are not supported: %q", path)
	}
	return FontSource{Path: path}, nil
}

func trimQuotes(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
