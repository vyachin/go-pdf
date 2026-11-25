package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/html"
)

// Options controls validation and stylesheet collection.
type Options struct {
	// BaseDir is the directory used to resolve relative stylesheet paths and
	// to confine them. When empty, paths resolve from the working directory.
	BaseDir string
	// Strict turns profile warnings into errors.
	Strict bool
	// Budget records collection and validation work and reports cancellation.
	// Nil means unlimited work and no cancellation.
	Budget *budget.Guard
	// MaxStylesheetBytes bounds one external stylesheet. Zero means unlimited.
	MaxStylesheetBytes int
}

// linkError carries a diagnostic code for a bad stylesheet reference.
type linkError struct {
	code string
	msg  string
	// pos overrides the link element's position when the problem is inside
	// the stylesheet file rather than in the document that refers to it.
	pos *diag.Position
}

func (e *linkError) Error() string { return e.msg }

// CollectStylesheets walks the document in order and returns every stylesheet
// it finds: inline <style> elements and local <link rel="stylesheet"> files.
// The slice order is the cascade order.
func CollectStylesheets(doc *html.Document, opts Options) ([]*css.Stylesheet, diag.List) {
	var sheets []*css.Stylesheet
	var out diag.List
	root, rootErr := fsx.Open(opts.BaseDir)
	if rootErr != nil {
		out.Errorf("css/link-load", diag.Position{}, "", "cannot open the base directory: %v", rootErr)
	} else {
		defer func() { _ = root.Close() }()
	}
	stopped := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if stopped {
			return
		}
		if err := opts.Budget.Add(1); err != nil {
			stopped = true
			return
		}
		if n.Type == html.ElementNode {
			switch n.Tag {
			case "style":
				if !mediaAppliesToPrint(n) {
					return
				}
				base := n.Start
				if len(n.Children) > 0 && n.Children[0].Type == html.TextNode {
					base = n.Children[0].Start
				}
				src := []byte(n.TextContent())
				// One operation per token keeps the shared work budget honest
				// for inline CSS, which is bounded by the HTML input limit only.
				if err := opts.Budget.Add(uint64(len(src)/4) + 1); err != nil {
					stopped = true
					return
				}
				sheets = append(sheets, css.ParseSheet(css.SheetOptions{
					Base: base,
					Locate: func(offset int) css.Position {
						pos := doc.Position(offset)
						return css.Position{File: pos.File, Line: pos.Line, Col: pos.Col}
					},
				}, src))
				return
			case "link":
				sheet, err := loadStylesheet(doc, n, opts, root)
				if err != nil {
					var le *linkError
					if errors.As(err, &le) {
						pos := doc.Pos(n)
						if le.pos != nil {
							pos = *le.pos
						}
						out.Errorf(le.code, pos, n.Path(), "%s", le.msg)
						return
					}
					if errors.Is(err, budget.ErrExceeded) {
						out.Errorf("resource/limit", doc.Pos(n), n.Path(), "%v", err)
						stopped = true
						return
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						// The caller returns the context error; a link failure
						// diagnostic would misdescribe a cancelled read.
						stopped = true
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
			if stopped {
				return
			}
			walk(c)
		}
	}
	if doc.Root != nil {
		walk(doc.Root)
	}
	out.Sort()
	return sheets, out
}

// mediaAppliesToPrint reports whether a style or link element applies to
// print. A missing attribute, "print" and "all" apply; a list applies when any
// of its entries does; anything else belongs to another medium and is skipped
// without an error, the same way a browser skips it for print.
func mediaAppliesToPrint(n *html.Node) bool {
	media, _ := n.Attr("media")
	for _, part := range strings.Split(media, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "", "print", "all":
			return true
		}
	}
	return false
}

// loadStylesheet resolves one link element. A non stylesheet rel, or a media
// query that does not apply to print, returns nil without an error.
func loadStylesheet(doc *html.Document, n *html.Node, opts Options, root *fsx.Root) (*css.Stylesheet, error) {
	rel, _ := n.Attr("rel")
	if !strings.EqualFold(strings.TrimSpace(rel), "stylesheet") {
		return nil, nil
	}
	if !mediaAppliesToPrint(n) {
		return nil, nil
	}
	href, _ := n.Attr("href")
	path, err := resolveStylesheetPath(href, opts.BaseDir)
	if err != nil {
		return nil, err
	}
	data, err := readConfined(root, path, opts.MaxStylesheetBytes)
	if err != nil {
		return nil, err
	}
	// A stylesheet is bounded by MaxStylesheetBytes; the estimate still spends
	// work so that many small sheets cannot bypass the shared budget.
	if err := opts.Budget.Add(uint64(len(data)/4) + 1); err != nil {
		return nil, err
	}
	// A linked sheet is read with the document's encoding: CSS 2.1 gives a
	// sheet without its own @charset the encoding of the referring document.
	text, mapper, err := decodeSheet(doc.Charset(), path, data)
	if err != nil {
		return nil, err
	}
	opts2 := css.SheetOptions{File: path}
	if mapper != nil {
		// Positions in the decoded sheet are translated back to its source
		// bytes, so a diagnostic names the file the author edits.
		opts2.Locate = func(offset int) css.Position {
			line, col := mapper.Position(offset)
			return css.Position{File: path, Line: line, Col: col}
		}
	}
	return css.ParseSheet(opts2, text), nil
}

// decodeSheet decodes one linked stylesheet and consumes a @charset rule at its
// start. A rule that names the document's encoding is accepted and blanked, so
// that the parser sees whitespace where the rule was and every later offset
// stays where it was; a rule that names another encoding is an error. The
// returned mapper is nil when the decoded text is the source byte for byte.
func decodeSheet(enc *charset.Encoding, path string, data []byte) ([]byte, *charset.Mapper, error) {
	dec, err := charset.Decode(enc, data)
	if err != nil {
		var encErr *charset.Error
		if errors.As(err, &encErr) {
			pos := diag.Position{File: path, Line: encErr.Line, Col: encErr.Col}
			return nil, nil, &linkError{
				code: "css/link-encoding",
				msg:  fmt.Sprintf("cannot decode stylesheet %q: %s", path, encErr.Msg),
				pos:  &pos,
			}
		}
		return nil, nil, err
	}
	documentEncoding := charset.UTF8
	if enc != nil {
		documentEncoding = enc.Name()
	}
	text := dec.Text
	if label, end, ok := charsetRule(text); ok {
		named, err := charset.Validate(label)
		if err != nil || named.Name() != documentEncoding {
			pos := diag.Position{File: path, Line: 1, Col: 1}
			return nil, nil, &linkError{
				code: "css/link-encoding",
				msg:  fmt.Sprintf("stylesheet %q declares @charset %q but the document encoding is %q", path, label, documentEncoding),
				pos:  &pos,
			}
		}
		// Blank the rule instead of removing it: same length, same line
		// structure, the parser sees whitespace, and every later offset stays
		// where the author wrote it.
		text = bytes.Clone(text)
		for i := 0; i < end; i++ {
			if text[i] != '\n' && text[i] != '\r' {
				text[i] = ' '
			}
		}
	}
	if dec.Widths == nil && dec.Skip == 0 {
		// The decoded text is the source byte for byte, so offsets already
		// point into the file the author edits.
		return text, nil, nil
	}
	return text, charset.NewMapper(data, text, dec.Widths, dec.Skip), nil
}

// charsetRule returns the encoding label of a @charset rule at the very start
// of a stylesheet and the offset just past the rule. The rule must be the first
// thing in the file, as CSS requires.
func charsetRule(text []byte) (label string, end int, ok bool) {
	const prefix = "@charset"
	if len(text) < len(prefix) || !strings.EqualFold(string(text[:len(prefix)]), prefix) {
		return "", 0, false
	}
	i := len(prefix)
	for i < len(text) && isCSSSpace(text[i]) {
		i++
	}
	if i >= len(text) || (text[i] != '"' && text[i] != '\'') {
		return "", 0, false
	}
	quote := text[i]
	j := i + 1
	for j < len(text) && text[j] != quote {
		j++
	}
	if j >= len(text) {
		return "", 0, false
	}
	label = string(text[i+1 : j])
	j++
	for j < len(text) && isCSSSpace(text[j]) {
		j++
	}
	if j >= len(text) || text[j] != ';' {
		return "", 0, false
	}
	return label, j + 1, true
}

// isCSSSpace reports whether b is CSS whitespace.
func isCSSSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

// readConfined reads a stylesheet through a root confined to the base
// directory and maps failures to the diagnostics the profile reports for links.
func readConfined(root *fsx.Root, path string, limit int) ([]byte, error) {
	data, err := root.ReadLimit(path, limit)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, fsx.ErrTooLarge) {
		return nil, fmt.Errorf("%w: stylesheet %q exceeds the limit of %d bytes", budget.ErrExceeded, path, limit)
	}
	if errors.Is(err, fsx.ErrOutsideRoot) {
		return nil, &linkError{code: "css/link-path", msg: fmt.Sprintf("stylesheet path %q escapes the base directory", path)}
	}
	return nil, &linkError{code: "css/link-load", msg: fmt.Sprintf("cannot read stylesheet %q: %v", path, err)}
}

// resolveStylesheetPath turns an href into a readable local path. Remote and
// data: URLs are rejected here; whether the path stays inside the base
// directory is decided by the caller when it reads through fsx.Root, the same
// rule the image loader uses.
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
	// The absolute form is the stable name a diagnostic and a stylesheet
	// position report; the boundary is not decided here but by fsx.Root.
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", &linkError{code: "css/link-path", msg: fmt.Sprintf("cannot resolve stylesheet path: %v", err)}
	}
	return abs, nil
}
