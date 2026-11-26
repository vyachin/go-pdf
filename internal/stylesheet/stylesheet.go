// Package stylesheet loads and decodes the stylesheets a document references.
//
// It is the stage between the CSS parser and profile validation: it walks the
// document in cascade order, reads every local <link rel="stylesheet"> through
// the conversion's asset registry, decodes the bytes with the document's
// encoding, consumes a leading @charset rule and returns the parsed sheets.
// Validation stays in internal/profile, which reads no asset and opens no file.
package stylesheet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/html"
)

// Options controls stylesheet collection.
type Options struct {
	// BaseDir is the directory used to resolve relative stylesheet paths and
	// to confine them. When empty, paths resolve from the working directory.
	BaseDir string
	// Budget records collection work and reports cancellation. Nil means
	// unlimited work and no cancellation.
	Budget *budget.Guard
	// MaxStylesheetBytes bounds one external stylesheet. Zero means unlimited.
	MaxStylesheetBytes int
	// MaxTotalStylesheetBytes bounds the encoded bytes of every linked
	// stylesheet one conversion reads. Zero means unlimited.
	MaxTotalStylesheetBytes int
	// Assets is the conversion's asset registry. A nil registry makes Collect
	// create and close its own, which is what a standalone collection does.
	Assets *assets.Registry
}

// linkError carries a diagnostic code for a bad stylesheet reference.
type linkError struct {
	code diag.Code
	msg  string
	// pos overrides the link element's position when the problem is inside
	// the stylesheet file rather than in the document that refers to it.
	pos *diag.Position
	// cause is the failure the diagnostic describes, when one is known. It is
	// kept so a caller can still match a typed cause — fsx.ErrNotRegular, for
	// example — instead of receiving only a message.
	cause error
}

func (e *linkError) Error() string { return e.msg }

// Unwrap returns the failure the diagnostic describes, when one is known.
func (e *linkError) Unwrap() error { return e.cause }

// Collect walks the document in order and returns every stylesheet it finds:
// inline <style> elements and local <link rel="stylesheet"> files. The slice
// order is the cascade order.
func Collect(doc *html.Document, opts Options) ([]*css.Stylesheet, diag.List) {
	var sheets []*css.Stylesheet
	var out diag.List
	reg := opts.Assets
	if reg == nil {
		// A standalone collection owns its registry and closes it here; a
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
				sheet, key, err := loadSheet(doc, n, opts, reg)
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

// CollectInline returns the sheets of the document's <style> elements without
// opening a linked asset. A request whose options are already invalid uses it
// to report the document's own CSS in the same pass, as the library contract
// requires.
func CollectInline(doc *html.Document, opts Options) ([]*css.Stylesheet, diag.List) {
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

// loadSheet resolves one link element. A non stylesheet rel, or a media query
// that does not apply to print, returns nil without an error. It also returns
// the record identity, so a repeated inclusion reuses the immutable parse while
// keeping its place in the cascade.
//
// The registry is the only path resolver: the loader passes the reference as
// the document wrote it and never resolves or joins it itself.
func loadSheet(doc *html.Document, n *html.Node, opts Options, reg *assets.Registry) (*css.Stylesheet, string, error) {
	rel, _ := n.Attr("rel")
	if !strings.EqualFold(strings.TrimSpace(rel), "stylesheet") {
		return nil, "", nil
	}
	if !mediaAppliesToPrint(n) {
		return nil, "", nil
	}
	href, _ := n.Attr("href")
	// The document encoding is part of the identity: one file decoded with two
	// encodings is two records.
	decoding := ""
	if enc := doc.Charset(); enc != nil {
		decoding = enc.Name()
	}
	rec, err := reg.Read(assets.Request{
		Kind:          assets.Stylesheet,
		Reference:     href,
		Dir:           opts.BaseDir,
		MaxBytes:      opts.MaxStylesheetBytes,
		MaxTotalBytes: int64(opts.MaxTotalStylesheetBytes),
		Decoding:      decoding,
	})
	if err != nil {
		return nil, "", readError(err, href)
	}
	path := rec.Path
	// A linked sheet is read with the document's encoding: CSS 2.1 gives a
	// sheet without its own @charset the encoding of the referring document.
	text, mapper, err := decode(doc.Charset(), path, rec.Bytes, opts.Budget)
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

// readError maps one registry failure to the link diagnostic the profile
// reports. The registry already resolved the reference, so the loader maps its
// typed failure instead of resolving the same reference again.
func readError(err error, href string) error {
	var assetErr *assets.Error
	if errors.As(err, &assetErr) {
		switch {
		case errors.Is(assetErr.Err, fsx.ErrOutsideRoot):
			return &linkError{code: diag.CSSLinkPath, msg: fmt.Sprintf("stylesheet path %q escapes the base directory", href)}
		case errors.Is(assetErr.Err, fsx.ErrNotRegular):
			// The reference names something that is not a file. Name it as the
			// document wrote it, like a path escape, and keep the typed cause
			// for a caller that matches the failure.
			return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("cannot read stylesheet %q: %v", href, assetErr.Err), cause: assetErr.Err}
		case errors.Is(assetErr.Err, assets.ErrTooLarge):
			return fmt.Errorf("%w: stylesheet %q exceeds the limit", budget.ErrExceeded, href)
		case errors.Is(assetErr.Err, budget.ErrExceeded):
			// The aggregate total or the byte charge refused the read: report
			// the limit itself, not a malformed link.
			return fmt.Errorf("%w: stylesheet %q: %v", budget.ErrExceeded, href, assetErr.Err)
		case errors.Is(assetErr.Err, context.Canceled), errors.Is(assetErr.Err, context.DeadlineExceeded):
			// Cancellation ends the conversion; a link diagnostic would
			// misdescribe a read the caller stopped.
			return assetErr.Err
		}
		if assetErr.Path != "" {
			return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("cannot read stylesheet %q: %v", assetErr.Path, assetErr.Err), cause: assetErr.Err}
		}
	}
	if refErr := referenceError(href); refErr != nil {
		return refErr
	}
	return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("cannot read stylesheet %q: %v", href, err), cause: err}
}

// referenceError names the references that cannot denote a local file. The
// registry classifies the same forms before it opens anything, but its typed
// failure keeps that classification opaque; repeating it after a failed read
// keeps the codes and messages this stage has always reported without
// resolving the reference to a path. The empty and no-file checks mirror the
// shared URL resolver, including its messages.
func referenceError(href string) error {
	s := strings.TrimSpace(href)
	if s == "" {
		return &linkError{code: diag.CSSLinkLoad, msg: "stylesheet link has no href"}
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "data:"):
		return &linkError{code: diag.CSSLinkUnsupported, msg: "data: stylesheets are not supported"}
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(s, "//"), strings.HasPrefix(lower, "ftp://"):
		return &linkError{code: diag.CSSLinkRemote, msg: fmt.Sprintf("remote stylesheets are not supported: %s", s)}
	}
	if strings.IndexAny(s, "?#") == 0 {
		return &linkError{code: diag.CSSLinkLoad, msg: fmt.Sprintf("asset reference %q names no file", s)}
	}
	return nil
}
