package profile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// Options controls validation and stylesheet collection.
type Options struct {
	// BaseDir is the directory used to resolve relative stylesheet paths and
	// to confine them. When empty, paths resolve from the working directory.
	BaseDir string
	// Strict turns profile warnings into errors.
	Strict bool
}

// linkError carries a diagnostic code for a bad stylesheet reference.
type linkError struct {
	code string
	msg  string
}

func (e *linkError) Error() string { return e.msg }

// CollectStylesheets walks the document in order and returns every stylesheet
// it finds: inline <style> elements and local <link rel="stylesheet"> files.
// The slice order is the cascade order.
func CollectStylesheets(doc *html.Document, opts Options) ([]*css.Stylesheet, diag.List) {
	var sheets []*css.Stylesheet
	var out diag.List
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Tag {
			case "style":
				base := n.Start
				if len(n.Children) > 0 && n.Children[0].Type == html.TextNode {
					base = n.Children[0].Start
				}
				sheets = append(sheets, css.ParseSheet(css.SheetOptions{
					Base: base,
					Locate: func(offset int) css.Position {
						pos := doc.Position(offset)
						return css.Position{File: pos.File, Line: pos.Line, Col: pos.Col}
					},
				}, []byte(n.TextContent())))
				return
			case "link":
				sheet, err := loadStylesheet(doc, n, opts)
				if err != nil {
					var le *linkError
					if ok := asLinkError(err, &le); ok {
						out.Errorf(le.code, doc.Pos(n), n.Path(), "%s", le.msg)
						return
					}
					out.Errorf("css/link-load", doc.Pos(n), n.Path(), "%v", err)
					return
				}
				if sheet != nil {
					sheets = append(sheets, sheet)
				}
				return
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if doc.Root != nil {
		walk(doc.Root)
	}
	out.Sort()
	return sheets, out
}

func asLinkError(err error, target **linkError) bool {
	for err != nil {
		if le, ok := err.(*linkError); ok {
			*target = le
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// loadStylesheet resolves one link element. A non stylesheet rel, or a media
// query that does not apply to print, returns nil without an error.
func loadStylesheet(doc *html.Document, n *html.Node, opts Options) (*css.Stylesheet, error) {
	rel, _ := n.Attr("rel")
	if !strings.EqualFold(strings.TrimSpace(rel), "stylesheet") {
		return nil, nil
	}
	media, _ := n.Attr("media")
	switch strings.ToLower(strings.TrimSpace(media)) {
	case "", "print", "all":
	default:
		return nil, nil
	}
	href, _ := n.Attr("href")
	path, err := resolveStylesheetPath(href, opts.BaseDir)
	if err != nil {
		return nil, err
	}
	data, err := readConfined(path, opts.BaseDir)
	if err != nil {
		return nil, err
	}
	return css.ParseSheet(css.SheetOptions{File: path}, data), nil
}

// ErrOutsideRoot reports a local asset path that leaves its configured root.
var ErrOutsideRoot = errors.New("path escapes the root directory")

// ReadWithin reads a local asset that must stay inside rootDir. The path and
// the root are resolved first, so symbolic links that point inside the root
// keep working; the file is then opened through os.Root, which rejects
// anything that leaves the root at open time, including a link swapped in
// after the check. An empty rootDir disables the boundary.
func ReadWithin(rootDir, path string) ([]byte, error) {
	if rootDir == "" {
		return os.ReadFile(path)
	}
	baseReal, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", rootDir, err)
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(baseReal, pathReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrOutsideRoot
	}
	root, err := os.OpenRoot(baseReal)
	if err != nil {
		return nil, fmt.Errorf("opening root %q: %w", rootDir, err)
	}
	defer root.Close()
	f, err := root.Open(rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, ErrOutsideRoot
	}
	defer f.Close()
	return io.ReadAll(f)
}

// readConfined reads a stylesheet inside the base directory and maps failures
// to the diagnostics the profile reports for links.
func readConfined(path, baseDir string) ([]byte, error) {
	data, err := ReadWithin(baseDir, path)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, ErrOutsideRoot) {
		return nil, &linkError{code: "css/link-path", msg: fmt.Sprintf("stylesheet path %q escapes the base directory", path)}
	}
	return nil, &linkError{code: "css/link-load", msg: fmt.Sprintf("cannot read stylesheet %q: %v", path, err)}
}

// resolveStylesheetPath turns an href into a readable local path. Remote and
// data: URLs are rejected, and a configured base directory is a hard boundary.
func resolveStylesheetPath(href, baseDir string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", &linkError{code: "css/link-load", msg: "stylesheet link has no href"}
	}
	lower := strings.ToLower(href)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "//"):
		return "", &linkError{code: "css/link-remote", msg: fmt.Sprintf("remote stylesheets are not supported: %s", href)}
	case strings.HasPrefix(lower, "data:"):
		return "", &linkError{code: "css/link-unsupported", msg: "data: stylesheets are not supported"}
	}
	path := href
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)
	if baseDir == "" {
		return path, nil
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return "", &linkError{code: "css/link-path", msg: fmt.Sprintf("cannot resolve base directory: %v", err)}
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return "", &linkError{code: "css/link-path", msg: fmt.Sprintf("cannot resolve stylesheet path: %v", err)}
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &linkError{code: "css/link-path", msg: fmt.Sprintf("stylesheet path %q escapes the base directory", href)}
	}
	return target, nil
}
