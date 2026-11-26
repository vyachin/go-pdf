package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vyachin/go-pdf/internal/asset"
	"github.com/vyachin/go-pdf/internal/assets"
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
	// Assets is the conversion's asset registry. A nil registry makes the
	// profile create and close its own, which is what a standalone validation
	// does.
	Assets *assets.Registry
}

// linkError carries a diagnostic code for a bad stylesheet reference.
type linkError struct {
	code diag.Code
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
	reg := opts.Assets
	if reg == nil {
		// A standalone validation owns its registry and closes it here; a
		// conversion passes the registry it already opened.
		reg = assets.New(opts.Budget)
		defer func() { _ = reg.Close() }()
	}
	if reg.IsSource() {
		// A source root is a directory inside the caller's fs.FS, not a host
		// directory: there is no handle to open here, and a reference outside
		// it is rejected by the read that follows.
	} else if _, rootErr := reg.Root(opts.BaseDir); rootErr != nil {
		// A configured root that cannot be opened is a configuration error.
		// Continuing with a nil root would mean "no boundary" and read an
		// absolute link outside the directory the caller named.
		out.Errorf(diag.OptionsAssetRoot, diag.Position{}, "", "cannot open the base directory: %v", rootErr)
		return nil, out
	}
	// The registry owns the handle now; it closes it when the conversion ends.
	// A repeated stylesheet shares one read and one immutable parse, but every
	// inclusion keeps its own place in the cascade.
	parsed := map[string]*css.Stylesheet{}
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
				sheet := parseInlineSheet(doc, n, opts)
				if sheet == nil {
					stopped = true
					return
				}
				sheets = append(sheets, sheet)
				return
			case "link":
				sheet, key, err := loadStylesheet(doc, n, opts, reg)
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
						out.Errorf(diag.ResourceLimit, doc.Pos(n), n.Path(), "%v", err)
						stopped = true
						return
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						// The caller returns the context error; a link failure
						// diagnostic would misdescribe a cancelled read.
						stopped = true
						return
					}
					out.Errorf(diag.CSSLinkLoad, doc.Pos(n), n.Path(), "%v", err)
					return
				}
				if sheet != nil {
					if cached, ok := parsed[key]; ok {
						sheet = cached
					} else {
						parsed[key] = sheet
					}
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

// parseInlineSheet parses one <style> element, charging the work and keeping
// the document positions. It returns nil when the budget stopped the parse.
func parseInlineSheet(doc *html.Document, n *html.Node, opts Options) *css.Stylesheet {
	base := n.Start
	if len(n.Children) > 0 && n.Children[0].Type == html.TextNode {
		base = n.Children[0].Start
	}
	src := []byte(n.TextContent())
	// One operation per four bytes keeps the shared work budget honest for
	// inline CSS, which is bounded by the HTML input limit only.
	if err := opts.Budget.Add(uint64(len(src)/4) + 1); err != nil {
		return nil
	}
	return css.ParseSheet(css.SheetOptions{
		Guard: opts.Budget,
		Base:  base,
		Locate: func(offset int) css.Position {
			pos := doc.Position(offset)
			return css.Position{File: pos.File, Line: pos.Line, Col: pos.Col}
		},
	}, src)
}

// CollectInlineStylesheets returns the sheets of the document's <style>
// elements without opening a linked asset. A request whose options are already
// invalid uses it to report the document's own CSS in the same pass, as the
// library contract requires.
func CollectInlineStylesheets(doc *html.Document, opts Options) ([]*css.Stylesheet, diag.List) {
	var sheets []*css.Stylesheet
	var out diag.List
	stopped := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if stopped || n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Tag == "style" && mediaAppliesToPrint(n) {
			sheet := parseInlineSheet(doc, n, opts)
			if sheet == nil {
				stopped = true
				return
			}
			sheets = append(sheets, sheet)
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

// mediaAppliesToPrint reports whether a style or link element applies to
// print. A missing attribute, "print" and "all" apply; a list applies when any
// of its entries does; anything else belongs to another medium and is skipped
// without an error, the same way a browser skips it for print.
func mediaAppliesToPrint(n *html.Node) bool {
	media, _ := n.Attr("media")
	return css.MediaAppliesToPrint(media)
}

// loadStylesheet resolves one link element. A non stylesheet rel, or a media
// query that does not apply to print, returns nil without an error. It also
// returns the record identity, so a repeated inclusion reuses the immutable
// parse while keeping its place in the cascade.
func loadStylesheet(doc *html.Document, n *html.Node, opts Options, reg *assets.Registry) (*css.Stylesheet, string, error) {
	rel, _ := n.Attr("rel")
	if !strings.EqualFold(strings.TrimSpace(rel), "stylesheet") {
		return nil, "", nil
	}
	if !mediaAppliesToPrint(n) {
		return nil, "", nil
	}
	href, _ := n.Attr("href")
	if _, err := resolveStylesheetPath(href, opts.BaseDir, reg.IsSource()); err != nil {
		// The resolver's failures carry the link codes (remote, data:, empty);
		// its path is not needed because the registry resolves the reference.
		return nil, "", err
	}
	// The document encoding is part of the identity: one file decoded with two
	// encodings is two records.
	decoding := ""
	if enc := doc.Charset(); enc != nil {
		decoding = enc.Name()
	}
	rec, err := reg.Read(assets.Request{
		Kind:      assets.Stylesheet,
		Reference: href,
		Dir:       opts.BaseDir,
		MaxBytes:  opts.MaxStylesheetBytes,
		Decoding:  decoding,
	})
	if err != nil {
		return nil, "", stylesheetReadError(err, href)
	}
	path := rec.Path
	// A linked sheet is read with the document's encoding: CSS 2.1 gives a
	// sheet without its own @charset the encoding of the referring document.
	text, mapper, err := decodeSheet(doc.Charset(), path, rec.Bytes)
	if err != nil {
		reg.Discard(rec)
		return nil, "", err
	}
	opts2 := css.SheetOptions{File: path, Guard: opts.Budget}
	if mapper != nil {
		// Positions in the decoded sheet are translated back to its source
		// bytes, so a diagnostic names the file the author edits.
		opts2.Locate = func(offset int) css.Position {
			line, col := mapper.Position(offset)
			return css.Position{File: path, Line: line, Col: col}
		}
	}
	return css.ParseSheet(opts2, text), rec.Key, nil
}

// stylesheetReadError maps one registry failure to the link diagnostic the
// profile reports.
func stylesheetReadError(err error, href string) error {
	var assetErr *assets.Error
	if errors.As(err, &assetErr) {
		switch {
		case errors.Is(assetErr.Err, fsx.ErrOutsideRoot):
			return &linkError{code: diag.CSSLinkPath, msg: fmt.Sprintf("stylesheet path %q escapes the base directory", href)}
		case errors.Is(assetErr.Err, assets.ErrTooLarge):
			return fmt.Errorf("%w: stylesheet %q exceeds the limit", budget.ErrExceeded, href)
		}
		if assetErr.Path != "" {
			return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("cannot read stylesheet %q: %v", assetErr.Path, assetErr.Err)}
		}
	}
	return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("cannot read stylesheet %q: %v", href, err)}
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
				code: diag.CSSLinkEncoding,
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
				code: diag.CSSLinkEncoding,
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

// resolveStylesheetPath turns an href into a readable local path. Remote and
// data: URLs are rejected here; whether the path stays inside the base
// directory is decided by the caller when it reads through fsx.Root, the same
// rule the image loader uses. When source is true the path is a name inside the
// caller's fs.FS, so no host filesystem rule is applied to it: the registry
// validates and resolves it on the read.
func resolveStylesheetPath(href, baseDir string, source bool) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", &linkError{code: diag.CSSLinkLoad, msg: "stylesheet link has no href"}
	}
	// The reference is a URL path: the query, the fragment, percent encoding
	// and surrounding whitespace are handled by the shared resolver.
	ref, err := asset.Resolve(href)
	if err != nil {
		return "", &linkError{code: diag.CSSLinkLoad, msg: err.Error()}
	}
	switch {
	case ref.Remote:
		return "", &linkError{code: diag.CSSLinkRemote, msg: fmt.Sprintf("remote stylesheets are not supported: %s", href)}
	case ref.Data:
		return "", &linkError{code: diag.CSSLinkUnsupported, msg: "data: stylesheets are not supported"}
	}
	if source {
		return ref.Path, nil
	}
	path := ref.Path
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
		return "", &linkError{code: diag.CSSLinkPath, msg: fmt.Sprintf("cannot resolve stylesheet path: %v", err)}
	}
	return abs, nil
}
