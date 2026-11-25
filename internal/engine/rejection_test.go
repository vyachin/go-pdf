package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// browserCertificateSource returns the committed browser-oriented certificate.
// It is a rejection fixture: the closed profile does not accept it.
func browserCertificateSource(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(), "testdata", "browser", "certificate.html"))
	if err != nil {
		t.Fatalf("reading the browser fixture: %v", err)
	}
	return src
}

// TestBrowserCertificateIsRejected guards the removed compatibility layer: the
// certificate fails profile validation in both modes, produces no PDF and
// reports no browser/* code.
func TestBrowserCertificateIsRejected(t *testing.T) {
	src := browserCertificateSource(t)
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "strict"}[strict], func(t *testing.T) {
			opts := Options{File: "certificate.html", FallbackFont: testFontPath(t), Strict: strict}
			res, err := Convert(context.Background(), src, opts)
			if err == nil || len(res.PDF) != 0 || !res.Diagnostics.HasErrors() {
				t.Fatalf("the certificate was accepted: err=%v pdf=%d diags:\n%s", err, len(res.PDF), strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			for _, d := range res.Diagnostics {
				if strings.HasPrefix(d.Code, "browser/") {
					t.Fatalf("a removed browser code was emitted: %s", d.String())
				}
			}
			diags, verr := Validate(context.Background(), src, opts)
			if verr != nil || !diags.HasErrors() {
				t.Fatalf("Validate accepted the certificate: err=%v diags:\n%s", verr, strings.Join(diags.Lines(), "\n"))
			}
			for _, d := range diags {
				if strings.HasPrefix(d.Code, "browser/") {
					t.Fatalf("Validate emitted a removed browser code: %s", d.String())
				}
			}
		})
	}
}

// TestFormerlyImportableConstructsAreRejected replaces the import acceptance
// coverage: every construct the lowering used to rewrite is a normal profile
// error now, in ordinary and strict mode.
func TestFormerlyImportableConstructsAreRejected(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"flex column", `<style>div { display: flex; flex-direction: column }</style><div><p>x</p></div>`},
		{"absolute cover image", `<div style="position: relative; height: 100pt"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==" style="position: absolute; top: 0; left: 0; width: 100pt; height: 100pt"></div>`},
		{"border-box", `<div style="box-sizing: border-box; width: 100pt; border: 1em solid #000">x</div>`},
		{"rem unit", `<style>p { margin: 2rem }</style><p>x</p>`},
		{"text transform", `<style>p { text-transform: uppercase }</style><p>x</p>`},
		{"cosmetic property", `<style>p { box-shadow: 0 0 1pt #000 }</style><p>x</p>`},
		{"screen media", `<style>@media (max-width: 793px) { p { color: #000 } }</style><p>x</p>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, strict := range []bool{false, true} {
				opts := Options{File: "case.html", FallbackFont: testFontPath(t), Strict: strict}
				res, err := Convert(context.Background(), []byte(tc.src), opts)
				if err == nil || len(res.PDF) != 0 || !res.Diagnostics.HasErrors() {
					t.Fatalf("strict=%v: accepted: err=%v diags:\n%s", strict, err, strings.Join(res.Diagnostics.Lines(), "\n"))
				}
				for _, d := range res.Diagnostics {
					if strings.HasPrefix(d.Code, "browser/") {
						t.Fatalf("strict=%v: removed browser code %s", strict, d.String())
					}
				}
				diags, verr := Validate(context.Background(), []byte(tc.src), opts)
				if verr != nil || !diags.HasErrors() {
					t.Fatalf("strict=%v: Validate accepted: err=%v", strict, verr)
				}
			}
		})
	}
}
