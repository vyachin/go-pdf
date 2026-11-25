package layout

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/text"
)

// Line is one laid out line of inline content.
type Line struct {
	Items  []LineItem
	Height int32
	// Ascent is the distance from the top of the line box to the baseline.
	Ascent int32
	// Natural is the width of the content before justification.
	Natural int32
	// Indent is the text-indent applied to this line.
	Indent int32
	// ForcedBreak is true when the line ended at a <br> or the end of the block.
	ForcedBreak bool
	// Anchors are element ids whose content starts on this line.
	Anchors []string
}

// LineItemKind classifies an item inside a line.
type LineItemKind int

const (
	// TextItem is a shaped word.
	TextItem LineItemKind = iota
	// SpaceItem is a collapsible space that justification may stretch.
	SpaceItem
	// ImageItem is an inline image.
	ImageItem
)

// LineItem is one piece of a line.
type LineItem struct {
	Kind  LineItemKind
	Face  *FontFace
	Run   text.Run
	Color css.Color
	Style *css.Style
	Image *ImageData
	W, H  int32
}

type pieceKind int

const (
	wordPiece pieceKind = iota
	spacePiece
	imagePiece
	anchorPiece
	breakPiece
)

type piece struct {
	kind      pieceKind
	text      string
	src       srcRef
	style     *css.Style
	image     *ImageData
	preserve  bool   // a space that must not be collapsed or dropped
	anchor    string // element id that starts at this piece
	wrapAfter bool   // the word ends with a hyphen and allows a break after it
}

// shape maps text to glyphs under the conversion's budget. A guard failure is
// returned to the caller, which stops the build instead of reporting a glyph
// error for work the budget refused to do.
func (l *layouter) shape(face *FontFace, s string) (text.Run, error) {
	return text.ShapeWithOptions(face.Font, face.Size, s, text.ShapeOptions{Guard: l.ctx.Budget})
}

// fontStop reports whether a font error ends the layout. A documented bound —
// a per-file, aggregate or parser limit — becomes one resource/limit
// diagnostic and suppresses the dependent text/font message; a cancelled
// context is left to the caller. An unavailable face is not a stop: the caller
// reports it against the text that needed it.
func (l *layouter) fontStop(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, budget.ErrExceeded) {
		if !l.limitReported {
			l.limitReported = true
			l.diags.Errorf(diag.ResourceLimit, diag.Position{}, "", "%v", err)
		}
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// stopsOnGuard reports whether err stops the layout and records why. A budget
// failure becomes one resource/limit diagnostic; a cancellation is reported by
// the engine after Run returns.
func (l *layouter) stopsOnGuard(err error) bool {
	if errors.Is(err, budget.ErrExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		_ = l.stopReason()
		return true
	}
	return false
}

func (l *layouter) face(style *css.Style) (*FontFace, error) {
	key := fmt.Sprintf("%v|%d|%d|%d", style.FontFamily, style.FontWeight, style.FontStyle, style.FontSize)
	if f, ok := l.faces[key]; ok {
		return f, nil
	}
	if l.ctx.Fonts == nil {
		return nil, fmt.Errorf("no fonts available")
	}
	face, err := l.ctx.Fonts.Face(style.FontFamily, style.FontWeight, style.FontStyle == css.FontItalic, style.FontSize)
	if err != nil {
		return nil, err
	}
	l.faces[key] = face
	return face, nil
}

func (l *layouter) collectPieces(b *Box, out *[]piece) {
	for _, c := range b.Children {
		switch c.Kind {
		case Text:
			// The piece keeps the text node it came from: a diagnostic names the
			// node's first byte, because splitting and whitespace folding have
			// already moved the characters away from their raw offsets.
			src := l.sourceOf(c.Node)
			if c.Text == "\n" {
				*out = append(*out, piece{kind: breakPiece, src: src, style: c.Style})
				continue
			}
			pre := c.Style.WhiteSpace == css.WhiteSpacePre
			items := text.Split(c.Text, pre)
			for _, it := range items {
				switch it.Kind {
				case text.Newline:
					*out = append(*out, piece{kind: breakPiece, src: src, style: c.Style})
				case text.Space:
					*out = append(*out, piece{kind: spacePiece, src: src, style: c.Style, preserve: pre})
				default:
					*out = append(*out, piece{kind: wordPiece, text: it.Text, src: src, style: c.Style, wrapAfter: it.BreakAfter})
				}
			}
		case Image:
			// An id on the image itself is an anchor like any other inline id;
			// without it a table of contents points at page 0.
			if c.Node != nil {
				if id := c.Node.ID(); id != "" {
					*out = append(*out, piece{kind: anchorPiece, style: c.Style, anchor: id})
				}
			}
			*out = append(*out, piece{kind: imagePiece, src: l.sourceOf(c.Node), style: c.Style, image: c.Image})
		default:
			if c.Node != nil {
				if id := c.Node.ID(); id != "" {
					*out = append(*out, piece{kind: anchorPiece, style: c.Style, anchor: id})
				}
			}
			l.collectPieces(c, out)
		}
	}
	// Generated content of a::after participates in the line flow like any
	// other inline text of the anchor.
	if b.Node != nil && b.Node.Tag == "a" && len(b.Style.AfterContent) > 0 {
		src := l.generatedSource(b, b.Style.AfterContent)
		for _, item := range text.Split(l.generatedText(b), false) {
			if item.Kind == text.Space {
				*out = append(*out, piece{kind: spacePiece, src: src, style: b.Style})
				continue
			}
			*out = append(*out, piece{kind: wordPiece, text: item.Text, src: src, style: b.Style})
		}
	}
}

// generatedText resolves the a::after content of an anchor.
func (l *layouter) generatedText(b *Box) string {
	var sb strings.Builder
	for _, item := range b.Style.AfterContent {
		switch item.Kind {
		case css.ContentText:
			sb.WriteString(item.Text)
		case css.ContentPageNumber, css.ContentPageCount:
			// Counters are only meaningful in margin boxes.
		case css.ContentTarget:
			href, _ := b.Node.Attr(item.Text)
			id := strings.TrimPrefix(href, "#")
			if id != "" && l.doc != nil {
				l.doc.Targets[id] = true
			}
			if page, ok := l.targets[id]; ok && page > 0 {
				sb.WriteString(strconv.Itoa(page))
			} else {
				sb.WriteString("0")
			}
		}
	}
	return sb.String()
}

// buildLines lays out the inline content of a block into lines of the given
// width. Errors are reported as diagnostics and the offending word is skipped.
func (l *layouter) buildLines(b *Box, width int32) {
	// Lines depend on the content width alone, so a box measured at the width
	// it will be placed at keeps the lines the measurement built. Without this
	// a break-inside: avoid box re-splits every descendant at every nesting
	// level, which is quadratic in the depth.
	if b.built && b.builtWidth == width {
		return
	}
	if l.tick() != nil {
		return
	}
	var pieces []piece
	l.collectPieces(b, &pieces)
	// A percentage indent resolves against the containing block width, not
	// against this box's content width.
	indentBase := width
	if b.containing > 0 {
		indentBase = b.containing
	}
	if b.indentBase > 0 {
		indentBase = b.indentBase
	}
	indent := b.Style.TextIndent.Resolve(indentBase)

	var lines []Line
	cur := Line{}
	var x int32
	flush := func(forced bool) {
		if len(cur.Items) == 0 {
			if forced {
				cur.ForcedBreak = true
				cur.Height = b.Style.LineHeightMP()
				lines = append(lines, cur)
				cur = Line{}
				x = 0
			}
			return
		}
		cur.Natural = x
		cur.ForcedBreak = forced
		if cur.Height == 0 {
			cur.Height = b.Style.LineHeightMP()
		}
		lines = append(lines, cur)
		cur = Line{}
		x = 0
	}

	// canWrap records that the previous piece created a break opportunity: a
	// space or a forced break. An inline element boundary alone is not one, so
	// "ab<span>cd</span>ef" stays a single word.
	canWrap := false
	// Anchors wait until the first item they introduce is actually placed, so
	// an anchor before a word that wraps moves to the new line with it.
	var pendingAnchors []string
	attachAnchors := func() {
		if len(pendingAnchors) == 0 {
			return
		}
		cur.Anchors = append(cur.Anchors, pendingAnchors...)
		pendingAnchors = nil
	}

	firstLine := true
	// ensureIndent shifts the first line by text-indent before its first item,
	// whatever that item is. Applying it when the first word arrived used to
	// reset x and drop the width of a leading inline image.
	ensureIndent := func() {
		if firstLine && len(lines) == 0 && len(cur.Items) == 0 && indent != 0 {
			cur.Indent = indent
			x = indent
		}
	}

	// Collapsible spaces are held back until something follows them on the
	// same line, which collapses runs across text nodes and drops trailing
	// spaces without touching the line width afterwards.
	spacePending := false
	addSpace := func(style *css.Style, src srcRef) {
		ensureIndent()
		face, err := l.face(style)
		if err != nil {
			// After a budget failure the missing face is a consequence of the
			// work that was refused, not a fact about the document.
			if l.stopsOnGuard(l.ctx.Budget.Err()) {
				return
			}
			if l.fontStop(err) {
				return
			}
			pos, element := src.at()
			l.diags.Errorf(diag.TextFont, pos, element, "%v", err)
			return
		}
		run, err := l.shape(face, " ")
		if err != nil {
			_ = l.stopsOnGuard(err)
			return
		}
		item := LineItem{Kind: SpaceItem, Face: face, Run: run, Color: style.Color, Style: style, W: run.Width, H: style.LineHeightMP()}
		cur.Items = append(cur.Items, item)
		cur.Height = max(cur.Height, item.H)
		cur.Ascent = max(cur.Ascent, face.Ascent)
		x = satAdd(x, run.Width)
	}

	for _, p := range pieces {
		switch p.kind {
		case anchorPiece:
			if p.anchor != "" {
				pendingAnchors = append(pendingAnchors, p.anchor)
			}
		case breakPiece:
			spacePending = false
			canWrap = false
			attachAnchors()
			flush(true)
			firstLine = false
		case spacePiece:
			if p.preserve {
				attachAnchors()
				addSpace(p.style, p.src)
				canWrap = true
				continue
			}
			if len(cur.Items) > 0 {
				spacePending = true
				canWrap = true
			}
		case imagePiece:
			if p.image == nil {
				continue
			}
			ensureIndent()
			w, h := imageSize(p.style, p.image, width, l.ctx)
			// The pending separator belongs to the previous line only when the
			// image stays there; on a wrap it must not trail the old line.
			spaceW := int32(0)
			if spacePending && len(cur.Items) > 0 {
				spaceW = l.spaceWidth(p.style)
			}
			if len(cur.Items) > 0 && int64(x)+int64(spaceW)+int64(w) > int64(width) {
				spacePending = false
				flush(false)
				canWrap = false
				firstLine = false
			}
			if spacePending && len(cur.Items) > 0 {
				addSpace(p.style, p.src)
				spacePending = false
			}
			attachAnchors()
			item := LineItem{Kind: ImageItem, Image: p.image, W: w, H: h, Style: p.style}
			cur.Items = append(cur.Items, item)
			cur.Height = max(cur.Height, h)
			cur.Ascent = max(cur.Ascent, h)
			x = satAdd(x, w)
		case wordPiece:
			face, err := l.face(p.style)
			if err != nil {
				if l.stopsOnGuard(l.ctx.Budget.Err()) {
					return
				}
				if l.fontStop(err) {
					return
				}
				pos, element := p.src.at()
				l.diags.Errorf(diag.TextFont, pos, element, "%v", err)
				continue
			}
			run, err := l.shape(face, p.text)
			if err != nil {
				if l.stopsOnGuard(err) {
					return
				}
				pos, element := p.src.at()
				l.diags.Errorf(diag.TextGlyph, pos, element, "%v", err)
				continue
			}
			ensureIndent()
			// The word is measured together with the pending separator. On a
			// wrap that separator disappears, so the old line must not keep it.
			spaceW := int32(0)
			if spacePending && len(cur.Items) > 0 {
				spaceW = l.spaceWidth(p.style)
			}
			// Preformatted text wraps only on explicit line feeds; it may
			// overflow the content box instead.
			if len(cur.Items) > 0 && canWrap && x+spaceW+run.Width > width && p.style.WhiteSpace != css.WhiteSpacePre {
				spacePending = false
				flush(false)
				firstLine = false
			}
			if spacePending && len(cur.Items) > 0 {
				addSpace(p.style, p.src)
				spacePending = false
			}
			attachAnchors()
			item := LineItem{Kind: TextItem, Face: face, Run: run, Color: p.style.Color, Style: p.style, W: run.Width}
			cur.Items = append(cur.Items, item)
			cur.Height = max(cur.Height, p.style.LineHeightMP())
			cur.Ascent = max(cur.Ascent, face.Ascent)
			x = satAdd(x, run.Width)
			// A following word can wrap after a space, a forced break or a
			// hyphen that already ends this word.
			canWrap = p.wrapAfter
		}
	}
	// A trailing anchor belongs to the line it follows.
	attachAnchors()
	// A block without inline content must not produce a line; explicit breaks
	// already flushed their own.
	flush(false)

	for i := range lines {
		fitMetrics(&lines[i])
	}
	// Every built line is charged, so a document cannot buy an unbounded
	// amount of line building with a small budget.
	if err := l.ctx.Budget.Add(uint64(len(lines))); err != nil {
		_ = l.stopReason()
		return
	}
	b.lines = lines
	b.built = true
	b.builtWidth = width
}

// spaceWidth returns the advance of one collapsible space.
func (l *layouter) spaceWidth(style *css.Style) int32 {
	face, err := l.face(style)
	if err != nil {
		return 0
	}
	run, err := l.shape(face, " ")
	if err != nil {
		_ = l.stopsOnGuard(err)
		return 0
	}
	return run.Width
}

func fitMetrics(line *Line) {
	if line.Height == 0 {
		line.Height = line.Ascent
	}
	for _, it := range line.Items {
		if it.Kind == ImageItem {
			if line.Ascent < it.H {
				line.Ascent = it.H
			}
		}
	}
	if line.Ascent+line.Ascent/8 > line.Height {
		line.Height = line.Ascent + line.Ascent/4
	}
}

func imageSize(style *css.Style, img *ImageData, containerWidth int32, ctx Context) (int32, int32) {
	w, h := img.Width, img.Height
	specW, specH := style.Width, style.Height
	hasW := specW.Kind == css.MP || specW.Kind == css.Percent
	hasH := specH.Kind == css.MP || specH.Kind == css.Percent
	if hasW {
		w = specW.Resolve(containerWidth)
	}
	if hasH {
		h = specH.Resolve(containerWidth)
	}
	// The unspecified dimension follows the aspect ratio; this must work for
	// a percentage width as well as for an absolute one.
	if hasW && !hasH && img.Width > 0 {
		h = clampInt32(int64(w) * int64(img.Height) / int64(img.Width))
	}
	if hasH && !hasW && img.Height > 0 {
		w = clampInt32(int64(h) * int64(img.Width) / int64(img.Height))
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}
