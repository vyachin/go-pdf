package layout

import (
	"fmt"
	"strconv"
	"strings"

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
	kind     pieceKind
	text     string
	style    *css.Style
	node     *htmlNode
	image    *ImageData
	preserve bool   // a space that must not be collapsed or dropped
	anchor   string // element id that starts at this piece
}

// htmlNode is a minimal indirection so that diagnostics can carry a position
// without importing html into every helper.
type htmlNode struct {
	path string
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
			if c.Text == "\n" {
				*out = append(*out, piece{kind: breakPiece, style: c.Style})
				continue
			}
			pre := c.Style.WhiteSpace == css.WhiteSpacePre
			items := text.Split(c.Text, pre)
			for _, it := range items {
				switch it.Kind {
				case text.Newline:
					*out = append(*out, piece{kind: breakPiece, style: c.Style})
				case text.Space:
					*out = append(*out, piece{kind: spacePiece, style: c.Style, preserve: pre})
				default:
					*out = append(*out, piece{kind: wordPiece, text: it.Text, style: c.Style})
				}
			}
		case Image:
			*out = append(*out, piece{kind: imagePiece, style: c.Style, image: c.Image})
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
		for _, item := range text.Split(l.generatedText(b), false) {
			if item.Kind == text.Space {
				*out = append(*out, piece{kind: spacePiece, style: b.Style})
				continue
			}
			*out = append(*out, piece{kind: wordPiece, text: item.Text, style: b.Style})
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
	var pieces []piece
	l.collectPieces(b, &pieces)
	indent := b.Style.TextIndent.Resolve(width)
	if b.Style.TextIndent.Kind == css.Percent {
		indent = b.Style.TextIndent.Resolve(width)
	}

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

	firstLine := true
	ensureIndent := func() {
		if firstLine && len(lines) == 0 && indent > 0 {
			cur.Indent = indent
			x = indent
		}
	}

	// Collapsible spaces are held back until something follows them on the
	// same line, which collapses runs across text nodes and drops trailing
	// spaces without touching the line width afterwards.
	spacePending := false
	addSpace := func(style *css.Style) {
		face, err := l.face(style)
		if err != nil {
			l.diags.Errorf("text/font", diag.Position{}, "", "%v", err)
			return
		}
		run, err := text.Shape(face.Font, face.Size, " ")
		if err != nil {
			return
		}
		item := LineItem{Kind: SpaceItem, Face: face, Run: run, Color: style.Color, Style: style, W: run.Width, H: style.LineHeightMP()}
		cur.Items = append(cur.Items, item)
		cur.Height = max32(cur.Height, item.H)
		cur.Ascent = max32(cur.Ascent, face.Ascent)
		x += run.Width
	}

	for _, p := range pieces {
		switch p.kind {
		case anchorPiece:
			if p.anchor != "" {
				cur.Anchors = append(cur.Anchors, p.anchor)
			}
		case breakPiece:
			spacePending = false
			flush(true)
			firstLine = false
		case spacePiece:
			if p.preserve {
				addSpace(p.style)
				continue
			}
			if len(cur.Items) > 0 {
				spacePending = true
			}
		case imagePiece:
			if p.image == nil {
				continue
			}
			if spacePending && len(cur.Items) > 0 {
				addSpace(p.style)
				spacePending = false
			}
			w, h := imageSize(p.style, p.image, width, l.ctx)
			if len(cur.Items) > 0 && x+w > width {
				flush(false)
				firstLine = false
			}
			item := LineItem{Kind: ImageItem, Image: p.image, W: w, H: h, Style: p.style}
			cur.Items = append(cur.Items, item)
			cur.Height = max32(cur.Height, h)
			cur.Ascent = max32(cur.Ascent, h)
			x += w
		case wordPiece:
			face, err := l.face(p.style)
			if err != nil {
				l.diags.Errorf("text/font", diag.Position{}, "", "%v", err)
				continue
			}
			if spacePending && len(cur.Items) > 0 {
				addSpace(p.style)
				spacePending = false
			}
			run, err := text.Shape(face.Font, face.Size, p.text)
			if err != nil {
				l.diags.Errorf("text/glyph", diag.Position{}, "", "%v", err)
				continue
			}
			// Preformatted text wraps only on explicit line feeds; it may
			// overflow the content box instead.
			if len(cur.Items) > 0 && x+run.Width > width && p.style.WhiteSpace != css.WhiteSpacePre {
				flush(false)
				firstLine = false
			}
			ensureIndent()
			item := LineItem{Kind: TextItem, Face: face, Run: run, Color: p.style.Color, Style: p.style, W: run.Width}
			cur.Items = append(cur.Items, item)
			cur.Height = max32(cur.Height, p.style.LineHeightMP())
			cur.Ascent = max32(cur.Ascent, face.Ascent)
			x += run.Width
			_ = firstLine
		}
	}
	// A block without inline content must not produce a line; explicit breaks
	// already flushed their own.
	flush(false)

	for i := range lines {
		fitMetrics(&lines[i])
	}
	b.lines = lines
	b.built = true
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
		h = int32(int64(w) * int64(img.Height) / int64(img.Width))
	}
	if hasH && !hasW && img.Height > 0 {
		w = int32(int64(h) * int64(img.Width) / int64(img.Height))
	}
	return w, h
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
