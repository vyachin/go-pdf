package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/testfifo"
)

// fontRootDocument declares a font by the relative name body.ttf, so the
// conversion succeeds exactly when the effective font root holds that file.
const fontRootDocument = `<style>@font-face { font-family: "Body"; src: url(body.ttf) }
body { font-family: "Body" }</style><p>root</p>`

// TestRelativeFontRootPrecedence checks the four documented font roots with one
// relative reference: FontDir, then BaseDir, then the directory of Options.File,
// then the process working directory. Each directory holds only its own copy of
// the font, so a conversion can succeed only through the root under test.
func TestRelativeFontRootPrecedence(t *testing.T) {
	fontDirRoot := t.TempDir()
	baseDirRoot := t.TempDir()
	fileDirRoot := t.TempDir()
	cwdRoot := t.TempDir()
	for _, dir := range []string{fontDirRoot, baseDirRoot, fileDirRoot, cwdRoot} {
		copyTestFont(t, dir, "body.ttf")
	}

	cases := []struct {
		name string
		opts Options
		// chdir is the working directory the conversion runs in; empty keeps
		// the repository directory.
		chdir string
	}{
		{
			name: "configured FontDir",
			opts: Options{FontDir: fontDirRoot, File: "document.html"},
		},
		{
			name: "BaseDir when FontDir is empty",
			opts: Options{BaseDir: baseDirRoot, File: "document.html"},
		},
		{
			name: "directory of Options.File",
			opts: Options{File: filepath.Join(fileDirRoot, "document.html")},
		},
		{
			name:  "process working directory",
			opts:  Options{},
			chdir: cwdRoot,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.chdir != "" {
				t.Chdir(tc.chdir)
			}
			res, err := Convert(context.Background(), []byte(fontRootDocument), tc.opts)
			if err != nil {
				t.Fatalf("the relative font did not resolve through %s: %v\n%s", tc.name, err, strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			if len(res.PDF) == 0 {
				t.Fatalf("no PDF for %s", tc.name)
			}
		})
	}
}

// TestDocumentDirectoryIsTheDefaultImageRoot covers the delta scenario: with no
// BaseDir, a relative image resolves against the directory of Options.File.
func TestDocumentDirectoryIsTheDefaultImageRoot(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "logo.png"))
	res, err := Convert(context.Background(), []byte(`<p>x</p><img src="logo.png">`), Options{
		File:         filepath.Join(dir, "invoice.html"),
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("the document directory is not the default image root: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !strings.Contains(string(res.PDF), "/Subtype /Image") {
		t.Fatal("the adjacent image was not embedded")
	}
}

// TestFileLabelUsesTheWorkingDirectoryAsRoot covers the separation of the
// diagnostic name and the root: File stays the name in diagnostics while its
// directory supplies the default root, so a label such as "<stdin>" resolves
// relative assets in the working directory.
func TestFileLabelUsesTheWorkingDirectoryAsRoot(t *testing.T) {
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "logo.png"))
	t.Chdir(dir)

	res, err := Convert(context.Background(), []byte(`<img src="logo.png"><p>x</p>`), Options{
		File:         "<stdin>",
		FallbackFont: font,
	})
	if err != nil {
		t.Fatalf("the working directory is not the root of a labelled document: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !strings.Contains(string(res.PDF), "/Subtype /Image") {
		t.Fatal("the working-directory image was not embedded")
	}

	// The label is still the document name in a diagnostic.
	res, err = Convert(context.Background(), []byte(`<img src="missing.png">`), Options{
		File:         "<stdin>",
		FallbackFont: font,
	})
	if err == nil {
		t.Fatal("a missing image was accepted")
	}
	found := false
	for _, d := range res.Diagnostics {
		if string(d.Code) == "img/load" {
			found = true
			if d.Pos.File != "<stdin>" {
				t.Fatalf("img/load names %q, want the File label", d.Pos.File)
			}
		}
	}
	if !found {
		t.Fatalf("no img/load diagnostic: %v", res.Diagnostics.Lines())
	}
}

// writeOutsideAssets fills the directory above root with assets a relative
// escape would reach, so a rejected reference has something real to open.
func writeOutsideAssets(t *testing.T, parent string) {
	t.Helper()
	writeTestPNG(t, filepath.Join(parent, "secret.png"))
	if err := os.WriteFile(filepath.Join(parent, "secret.css"), []byte("p { color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyTestFont(t, parent, "secret.ttf")
}

// TestRelativeEscapeFromTheDocumentRoot checks every asset kind against a
// "../" reference that leaves the default document-directory root: the
// resource's load diagnostic is reported and no PDF is produced.
func TestRelativeEscapeFromTheDocumentRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "doc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOutsideAssets(t, parent)

	cases := []struct {
		kind string
		src  string
		code string
	}{
		{"image", `<p>x</p><img src="../secret.png">`, "img/load"},
		{"stylesheet", `<link rel="stylesheet" href="../secret.css"><p>x</p>`, "css/link-path"},
		{"font", `<style>@font-face { font-family: "X"; src: url(../secret.ttf) }
p { font-family: "X" }</style><p>x</p>`, "font/unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				File:         filepath.Join(root, "document.html"),
				FallbackFont: testFontPath(t),
			})
			if err == nil || len(res.PDF) != 0 {
				t.Fatalf("a %s escape produced a PDF: err=%v", tc.kind, err)
			}
			if !hasCode(res.Diagnostics, tc.code) {
				t.Fatalf("the %s escape was not reported as %s: %v", tc.kind, tc.code, res.Diagnostics.Lines())
			}
		})
	}
}

// TestEscapingSymlinkFromTheDocumentRoot checks every asset kind against a
// symlink inside the root that points outside it.
func TestEscapingSymlinkFromTheDocumentRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "doc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOutsideAssets(t, parent)
	links := map[string]string{
		"link.png": filepath.Join(parent, "secret.png"),
		"link.css": filepath.Join(parent, "secret.css"),
		"link.ttf": filepath.Join(parent, "secret.ttf"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("symlinks are not available: %v", err)
		}
	}

	cases := []struct {
		kind string
		src  string
		code string
	}{
		{"image", `<p>x</p><img src="link.png">`, "img/load"},
		{"stylesheet", `<link rel="stylesheet" href="link.css"><p>x</p>`, "css/link-path"},
		{"font", `<style>@font-face { font-family: "X"; src: url(link.ttf) }
p { font-family: "X" }</style><p>x</p>`, "font/unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				File:         filepath.Join(root, "document.html"),
				FallbackFont: testFontPath(t),
			})
			if err == nil || len(res.PDF) != 0 {
				t.Fatalf("a %s symlink escape produced a PDF: err=%v", tc.kind, err)
			}
			if !hasCode(res.Diagnostics, tc.code) {
				t.Fatalf("the %s symlink escape was not reported as %s: %v", tc.kind, tc.code, res.Diagnostics.Lines())
			}
		})
	}
}

// TestDefaultRootWithoutFileUsesWorkingDirectory covers the last delta
// scenario: both File and the roots are empty, so a relative reference resolves
// in the working directory and a "../" escape is rejected.
func TestDefaultRootWithoutFileUsesWorkingDirectory(t *testing.T) {
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "doc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(dir, "logo.png"))
	writeTestPNG(t, filepath.Join(parent, "secret.png"))
	t.Chdir(dir)

	res, err := Convert(context.Background(), []byte(`<img src="logo.png"><p>x</p>`), Options{FallbackFont: font})
	if err != nil {
		t.Fatalf("the working directory is not the default root: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !strings.Contains(string(res.PDF), "/Subtype /Image") {
		t.Fatal("the working-directory image was not embedded")
	}

	res, err = Convert(context.Background(), []byte(`<p>x</p><img src="../secret.png">`), Options{FallbackFont: font})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("a working-directory escape produced a PDF: err=%v", err)
	}
	if !hasCode(res.Diagnostics, "img/load") {
		t.Fatalf("the escape was not reported as img/load: %v", res.Diagnostics.Lines())
	}
}

// TestAbsoluteImagePolicy is the default/opt-in matrix for images: a configured
// root admits an absolute reference inside it and denies one outside, a rootless
// conversion rejects it, the opt-in reads it as written, and the opt-in does not
// override a configured root.
func TestAbsoluteImagePolicy(t *testing.T) {
	inside := t.TempDir()
	outside := t.TempDir()
	writeTestPNG(t, filepath.Join(inside, "logo.png"))
	outsideImage := filepath.Join(outside, "secret.png")
	writeTestPNG(t, outsideImage)
	doc := []byte(`<p>x</p><img src="` + outsideImage + `">`)
	font := testFontPath(t)

	t.Run("a configured root admits an absolute path inside it", func(t *testing.T) {
		absInside := filepath.Join(inside, "logo.png")
		res, err := Convert(context.Background(),
			[]byte(`<p>x</p><img src="`+absInside+`">`),
			Options{BaseDir: inside, File: "doc.html", FallbackFont: font})
		if err != nil {
			t.Fatalf("an absolute path inside BaseDir was rejected: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
		if !strings.Contains(string(res.PDF), "/Subtype /Image") {
			t.Fatal("the inside image was not embedded")
		}
	})

	t.Run("a configured root rejects an absolute path outside it", func(t *testing.T) {
		res, err := Convert(context.Background(), doc,
			Options{BaseDir: inside, File: "doc.html", FallbackFont: font})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("an absolute path outside BaseDir produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "img/load") {
			t.Fatalf("the outside image was not reported as img/load: %v", res.Diagnostics.Lines())
		}
	})

	t.Run("no configured root rejects an absolute path", func(t *testing.T) {
		res, err := Convert(context.Background(), doc,
			Options{File: "doc.html", FallbackFont: font})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("a rootless absolute path produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "img/load") {
			t.Fatalf("the rootless absolute image was not reported as img/load: %v", res.Diagnostics.Lines())
		}
	})

	t.Run("the opt-in reads an absolute path as written", func(t *testing.T) {
		res, err := Convert(context.Background(), doc,
			Options{File: "doc.html", AllowAbsoluteAssets: true, FallbackFont: font})
		if err != nil {
			t.Fatalf("the opt-in did not admit the absolute image: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
		if !strings.Contains(string(res.PDF), "/Subtype /Image") {
			t.Fatal("the opted-in image was not embedded")
		}
	})

	t.Run("the opt-in does not override a configured root", func(t *testing.T) {
		res, err := Convert(context.Background(), doc,
			Options{BaseDir: inside, File: "doc.html", AllowAbsoluteAssets: true, FallbackFont: font})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("the opt-in overrode a configured root: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "img/load") {
			t.Fatalf("the outside image was not reported as img/load: %v", res.Diagnostics.Lines())
		}
	})
}

// TestAbsoluteStylesheetAndFontPolicy is the same matrix for the two other
// kinds, plus the exact diagnostics the delta spec requires: css/link-path for a
// stylesheet and font/unavailable for a font.
func TestAbsoluteStylesheetAndFontPolicy(t *testing.T) {
	inside := t.TempDir()
	outside := t.TempDir()
	absInsideCSS := filepath.Join(inside, "print.css")
	if err := os.WriteFile(absInsideCSS, []byte("p { color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	absOutsideCSS := filepath.Join(outside, "secret.css")
	if err := os.WriteFile(absOutsideCSS, []byte("p { color: #00ff00 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	absInsideFont := filepath.Join(inside, "body.ttf")
	copyTestFont(t, inside, "body.ttf")
	absOutsideFont := filepath.Join(outside, "secret.ttf")
	copyTestFont(t, outside, "secret.ttf")
	font := testFontPath(t)

	stylesheetDoc := func(href string) []byte {
		return []byte(`<link rel="stylesheet" href="` + href + `"><p>x</p>`)
	}
	fontDoc := func(src string) []byte {
		return []byte(`<style>@font-face { font-family: "X"; src: url(` + src + `) }
p { font-family: "X" }</style><p>x</p>`)
	}

	t.Run("stylesheet inside a configured root", func(t *testing.T) {
		if _, err := Convert(context.Background(), stylesheetDoc(absInsideCSS),
			Options{BaseDir: inside, File: "doc.html", FallbackFont: font}); err != nil {
			t.Fatalf("an absolute stylesheet inside BaseDir was rejected: %v", err)
		}
	})
	t.Run("stylesheet outside a configured root", func(t *testing.T) {
		res, err := Convert(context.Background(), stylesheetDoc(absOutsideCSS),
			Options{BaseDir: inside, File: "doc.html", FallbackFont: font})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("an absolute stylesheet outside BaseDir produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "css/link-path") {
			t.Fatalf("the outside stylesheet was not reported as css/link-path: %v", res.Diagnostics.Lines())
		}
	})
	t.Run("stylesheet without a configured root", func(t *testing.T) {
		res, err := Convert(context.Background(), stylesheetDoc(absOutsideCSS),
			Options{File: "doc.html", FallbackFont: font})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("a rootless absolute stylesheet produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "css/link-path") {
			t.Fatalf("the rootless absolute stylesheet was not reported as css/link-path: %v", res.Diagnostics.Lines())
		}
	})
	t.Run("stylesheet opt-in", func(t *testing.T) {
		if _, err := Convert(context.Background(), stylesheetDoc(absOutsideCSS),
			Options{File: "doc.html", AllowAbsoluteAssets: true, FallbackFont: font}); err != nil {
			t.Fatalf("the opt-in did not admit the absolute stylesheet: %v", err)
		}
	})

	t.Run("font inside a configured root", func(t *testing.T) {
		if _, err := Convert(context.Background(), fontDoc(absInsideFont),
			Options{FontDir: inside, File: "doc.html"}); err != nil {
			t.Fatalf("an absolute font inside FontDir was rejected: %v", err)
		}
	})
	t.Run("font outside a configured root", func(t *testing.T) {
		res, err := Convert(context.Background(), fontDoc(absOutsideFont),
			Options{FontDir: inside, File: "doc.html"})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("an absolute font outside FontDir produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "font/unavailable") {
			t.Fatalf("the outside font was not reported as font/unavailable: %v", res.Diagnostics.Lines())
		}
	})
	t.Run("font without a configured root", func(t *testing.T) {
		res, err := Convert(context.Background(), fontDoc(absOutsideFont),
			Options{File: "doc.html"})
		if err == nil || len(res.PDF) != 0 {
			t.Fatalf("a rootless absolute font produced a PDF: err=%v", err)
		}
		if !hasCode(res.Diagnostics, "font/unavailable") {
			t.Fatalf("the rootless absolute font was not reported as font/unavailable: %v", res.Diagnostics.Lines())
		}
	})
	t.Run("font opt-in", func(t *testing.T) {
		if _, err := Convert(context.Background(), fontDoc(absOutsideFont),
			Options{File: "doc.html", AllowAbsoluteAssets: true}); err != nil {
			t.Fatalf("the opt-in did not admit the absolute font: %v", err)
		}
	})
}

// TestFallbackFontStaysExplicit covers the rule that the opt-in scopes document
// references only: the same absolute font path is rejected as a document
// reference and read as written as the caller-selected fallback.
func TestFallbackFontStaysExplicit(t *testing.T) {
	absFont, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	declared := `<style>@font-face { font-family: "Declared"; src: url(` + absFont + `) }
body { font-family: "Declared" }</style><p>x</p>`

	res, err := Convert(context.Background(), []byte(declared), Options{File: "doc.html"})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("an absolute document reference was accepted without a root: err=%v", err)
	}
	if !hasCode(res.Diagnostics, "font/unavailable") {
		t.Fatalf("the document reference was not reported as font/unavailable: %v", res.Diagnostics.Lines())
	}

	res, err = Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "doc.html",
		FallbackFont: absFont,
	})
	if err != nil {
		t.Fatalf("the explicit fallback was refused: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("the fallback conversion produced no PDF")
	}
}

// TestOptedInAbsoluteAssetKeepsTheRegularFileGate guards the coexistence of the
// confinement policy with the regular-file gate: the opt-in admits an absolute
// reference, but a path that is not a regular file is still refused by the
// resource's load diagnostic instead of being opened.
func TestOptedInAbsoluteAssetKeepsTheRegularFileGate(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "probe.fifo")
	testfifo.Create(t, fifo)
	font := testFontPath(t)

	cases := []struct {
		kind string
		src  string
		code string
	}{
		{"image", `<img src="` + fifo + `"><p>x</p>`, "img/load"},
		{"stylesheet", `<link rel="stylesheet" href="` + fifo + `"><p>x</p>`, "css/link-load"},
		{"font", `<style>@font-face { font-family: "Probe"; src: url(` + fifo + `) }
p { font-family: "Probe" }</style><p>x</p>`, "font/unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				File:                "opted-in.html",
				AllowAbsoluteAssets: true,
				FallbackFont:        font,
			})
			if err == nil || len(res.PDF) != 0 {
				t.Fatalf("an opted-in %s FIFO produced a PDF: err=%v", tc.kind, err)
			}
			if !hasCode(res.Diagnostics, tc.code) {
				t.Fatalf("the %s FIFO was not reported as %s: %v", tc.kind, tc.code, res.Diagnostics.Lines())
			}
			if !strings.Contains(strings.Join(res.Diagnostics.Lines(), "\n"), "not a regular file") {
				t.Fatalf("the %s diagnostic does not name a non-regular file: %v", tc.kind, res.Diagnostics.Lines())
			}
		})
	}
}
