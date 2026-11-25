// Package pdf writes PDF files: objects, fonts with a ToUnicode map, images
// and deterministic metadata.
package pdf

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

// RGB is a colour in the PDF device colour space.
type RGB struct {
	R, G, B uint8
	Set     bool
}

// Rect is a rectangle in millipoints with the origin at the top left.
type Rect struct {
	X, Y, W, H int32
}

// OpKind classifies a drawing operation.
type OpKind int

const (
	// FillOpKind fills a rectangle.
	FillOpKind OpKind = iota
	// TextOpKind draws glyphs.
	TextOpKind
	// ImageOpKind draws an image into a rectangle.
	ImageOpKind
)

// Glyph is a glyph id together with the character it came from, which is what
// makes a searchable text layer possible, and the advance calculated by layout,
// which is what preserves kerning in the PDF.
type Glyph struct {
	GID     uint16
	Rune    rune
	Advance int32
}

// Text is a run of glyphs drawn at a baseline.
type Text struct {
	X, Y   int32
	Font   *font.Font
	Size   int32
	Glyphs []Glyph
}

// Image is an embedded raster image.
type Image struct {
	Data   []byte
	Format string // "jpeg" or "png"
	PixelW int
	PixelH int
}

// Op is one drawing operation of a page.
type Op struct {
	Kind  OpKind
	Rect  Rect
	Color RGB
	Text  *Text
	Image *Image
	// Clip bounds the operation to a rectangle. A covered background image is
	// larger than its box on one axis, so the overflow is cut here; a nil clip
	// draws without clipping, which is what <img> needs.
	Clip *Rect
}

// OutlineEntry is one bookmark in the document outline.
type OutlineEntry struct {
	Title string
	Page  int
	Y     int32
	Level int
}

// Options configures document metadata.
type Options struct {
	Title        string
	Author       string
	Subject      string
	Keywords     string
	CreationDate time.Time
	Outline      []OutlineEntry
	// Budget records serialization work and reports cancellation. Nil means
	// unlimited work and no cancellation.
	Budget *budget.Guard
}

// DefaultCreationDate is used when no date is given, so that two runs of the
// same document stay byte identical.
var DefaultCreationDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Doc collects pages and writes them out.
type Doc struct {
	pages []*page
	opts  Options
}

type page struct {
	width, height int32
	ops           []Op
}

// New creates an empty document.
func New(opts Options) *Doc {
	if opts.CreationDate.IsZero() {
		opts.CreationDate = DefaultCreationDate
	}
	return &Doc{opts: opts}
}

// AddPage appends a page of the given size in millipoints.
func (d *Doc) AddPage(width, height int32, ops []Op) {
	d.pages = append(d.pages, &page{width: width, height: height, ops: ops})
}

type objectWriter struct {
	bodies [][]byte
}

func (w *objectWriter) add() int {
	w.bodies = append(w.bodies, nil)
	return len(w.bodies)
}

func (w *objectWriter) set(id int, body []byte) {
	w.bodies[id-1] = body
}

type fontEntry struct {
	font *font.Font
	used map[uint16]bool
	// pairs holds every (glyph, source rune) combination. A CID is allocated
	// per pair, so two characters that share a glyph keep their own code and
	// their own ToUnicode entry.
	pairs   map[uint64]struct{}
	cidOf   map[uint64]uint16
	cidGID  []uint16
	cidRune []rune
	subset  []byte
	ids     []int
}

// subsetTag returns the six-letter tag a subset font is named with. It is
// derived from the subset bytes, so the same font and the same glyph set always
// produce the same name.
func subsetTag(subset []byte) string {
	sum := sha256.Sum256(subset)
	tag := make([]byte, 6)
	for i := range tag {
		tag[i] = 'A' + sum[i]%26
	}
	return string(tag)
}

// numFloat formats a descriptor number without an exponent or a trailing zero.
func numFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// pairKey combines a glyph id and a source rune into one map key.
func pairKey(gid uint16, r rune) uint64 { return uint64(gid)<<32 | uint64(uint32(r)) }

// buildCIDs assigns one CID per distinct (glyph, rune) pair in a stable order.
func (fe *fontEntry) buildCIDs() {
	keys := make([]uint64, 0, len(fe.pairs))
	for k := range fe.pairs {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b uint64) int {
		if c := cmp.Compare(a>>32, b>>32); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	fe.cidOf = make(map[uint64]uint16, len(keys))
	fe.cidGID = make([]uint16, 0, len(keys))
	fe.cidRune = make([]rune, 0, len(keys))
	for i, k := range keys {
		cid := uint16(i)
		fe.cidOf[k] = cid
		fe.cidGID = append(fe.cidGID, uint16(k>>32))
		fe.cidRune = append(fe.cidRune, rune(uint32(k)))
	}
}

// cidToGIDMap is the CIDToGIDMap stream for the font.
func (fe *fontEntry) cidToGIDMap() []byte {
	out := make([]byte, 2*len(fe.cidGID))
	for i, gid := range fe.cidGID {
		out[2*i] = byte(gid >> 8)
		out[2*i+1] = byte(gid)
	}
	return out
}

type imageEntry struct {
	image *Image
	id    int
}

// Bytes serializes the document.
func (d *Doc) Bytes() ([]byte, error) {
	if err := d.opts.Budget.Err(); err != nil {
		return nil, err
	}
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("pdf: document has no pages")
	}
	// Fonts and images are collected in first-use order, which keeps the output
	// deterministic.
	var fonts []*fontEntry
	fontByPtr := map[*font.Font]*fontEntry{}
	for _, p := range d.pages {
		if err := d.opts.Budget.Add(uint64(len(p.ops)) + 1); err != nil {
			return nil, err
		}
		for _, op := range p.ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			if op.Text.Font == nil {
				return nil, fmt.Errorf("pdf: a text operation has no font")
			}
			fe := fontByPtr[op.Text.Font]
			if fe == nil {
				fe = &fontEntry{font: op.Text.Font, used: map[uint16]bool{}, pairs: map[uint64]struct{}{}}
				fontByPtr[op.Text.Font] = fe
				fonts = append(fonts, fe)
			}
			for _, g := range op.Text.Glyphs {
				fe.used[g.GID] = true
				fe.pairs[pairKey(g.GID, g.Rune)] = struct{}{}
			}
		}
	}
	var images []*imageEntry
	imageByPtr := map[*Image]*imageEntry{}
	for _, p := range d.pages {
		if err := d.opts.Budget.Add(uint64(len(p.ops)) + 1); err != nil {
			return nil, err
		}
		for _, op := range p.ops {
			if op.Kind != ImageOpKind || op.Image == nil {
				continue
			}
			if _, ok := imageByPtr[op.Image]; ok {
				continue
			}
			entry := &imageEntry{image: op.Image}
			imageByPtr[op.Image] = entry
			images = append(images, entry)
		}
	}

	w := &objectWriter{}
	// Font objects.
	for _, fe := range fonts {
		// Type0, CIDFontType2, FontDescriptor, FontFile2 and ToUnicode each
		// need their own object: reusing one id for two roles breaks the chain.
		fe.ids = make([]int, 6)
		for i := range fe.ids {
			fe.ids[i] = w.add()
		}
	}
	for _, im := range images {
		im.id = w.add()
	}
	contentIDs := make([]int, len(d.pages))
	pageIDs := make([]int, len(d.pages))
	for i := range d.pages {
		contentIDs[i] = w.add()
		pageIDs[i] = w.add()
	}
	pagesID := w.add()
	catalogID := w.add()
	infoID := w.add()

	fontNames := make([]string, len(fonts))
	for i := range fonts {
		fontNames[i] = fmt.Sprintf("F%d", i+1)
	}
	imageNames := map[*Image]string{}
	for i, im := range images {
		imageNames[im.image] = fmt.Sprintf("Im%d", i+1)
	}

	// Font objects.
	for _, fe := range fonts {
		if err := d.opts.Budget.Add(1); err != nil {
			return nil, err
		}
		subset, err := fe.font.Subset(fe.used)
		if err != nil {
			return nil, fmt.Errorf("pdf: subsetting font: %w", err)
		}
		fe.subset = subset
		if len(fe.pairs) > 0xFFFF {
			return nil, fmt.Errorf("%w: one font needs %d distinct glyph-rune pairs, the limit is 65535", budget.ErrExceeded, len(fe.pairs))
		}
		fe.buildCIDs()
		fontID, cidID := fe.ids[0], fe.ids[1]
		descID, fileID, toUniID := fe.ids[2], fe.ids[3], fe.ids[4]
		cidMapID := fe.ids[5]
		baseName := subsetTag(fe.subset) + "+" + sanitizeName(fe.font.Family(), fe.font.Subfamily())
		widths := fe.widthArray()
		toUni := fe.toUnicodeCMap()

		w.set(fontID, []byte(fmt.Sprintf(
			"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
			baseName, cidID, toUniID)))
		w.set(cidID, []byte(fmt.Sprintf(
			"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /DW 1000 /W [%s] /CIDToGIDMap %d 0 R >>",
			baseName, descID, widths, cidMapID)))
		upem := float64(fe.font.UnitsPerEm())
		scale := 1000 / upem
		bbox := fe.font.BBox()
		w.set(descID, []byte(fmt.Sprintf(
			"<< /Type /FontDescriptor /FontName /%s /Flags 4 /FontBBox [%d %d %d %d] /ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R >>",
			baseName,
			int32(float64(bbox[0])*scale), int32(float64(bbox[1])*scale),
			int32(float64(bbox[2])*scale), int32(float64(bbox[3])*scale),
			numFloat(fe.font.ItalicAngle()),
			int32(float64(fe.font.Ascent())*scale),
			int32(float64(fe.font.Descent())*scale),
			int32(float64(fe.font.CapHeight())*scale),
			fileID)))
		w.set(fileID, streamObject(fe.subset, true))
		w.set(toUniID, streamObject(toUni, true))
		w.set(cidMapID, streamObject(fe.cidToGIDMap(), true))
	}

	// Image objects. A PNG with transparency needs a second stream for its
	// soft mask; the allocator registers it and returns its object id.
	for _, im := range images {
		body, err := imageObject(im.image, func(mask []byte) int {
			id := w.add()
			w.set(id, mask)
			return id
		})
		if err != nil {
			return nil, err
		}
		w.set(im.id, body)
	}

	// Page content and page objects.
	for i, p := range d.pages {
		if err := d.opts.Budget.Add(uint64(len(p.ops)) + 1); err != nil {
			return nil, err
		}
		content := d.contentStream(p, fonts, fontNames, imageNames)
		w.set(contentIDs[i], streamObject(content, true))
		var fontRes, imageRes strings.Builder
		for fi, fe := range fonts {
			fmt.Fprintf(&fontRes, " /%s %d 0 R", fontNames[fi], fe.ids[0])
			_ = fe
		}
		// List only the images this page draws, in document order. Iterating the
		// ordered slice (not the name map) keeps the bytes deterministic.
		used := map[*Image]bool{}
		for _, op := range p.ops {
			if op.Kind == ImageOpKind && op.Image != nil {
				used[op.Image] = true
			}
		}
		for _, im := range images {
			if !used[im.image] {
				continue
			}
			fmt.Fprintf(&imageRes, " /%s %d 0 R", imageNames[im.image], im.id)
		}
		resources := fmt.Sprintf("/Font <<%s >>", fontRes.String())
		// Only images that the page actually uses need to be listed, but a
		// shared list keeps the writer simple and is valid.
		if imageRes.Len() > 0 {
			resources += fmt.Sprintf(" /XObject <<%s >>", imageRes.String())
		}
		w.set(pageIDs[i], []byte(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << %s >> /Contents %d 0 R >>",
			pagesID, num(p.width), num(p.height), resources, contentIDs[i])))
	}

	kids := make([]string, len(pageIDs))
	for i, id := range pageIDs {
		kids[i] = fmt.Sprintf("%d 0 R", id)
	}
	w.set(pagesID, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageIDs))))
	outlineRoot := 0
	if len(d.opts.Outline) > 0 {
		outlineRoot = w.add()
	}
	if outlineRoot != 0 {
		w.set(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R /PageMode /UseOutlines >>", pagesID, outlineRoot)))
		writeOutline(w, d.opts.Outline, outlineRoot, d.pages, pageIDs, outlineRoot)
	} else {
		w.set(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)))
	}

	keywords := ""
	if d.opts.Keywords != "" {
		keywords = " /Keywords " + pdfString(d.opts.Keywords)
	}
	info := fmt.Sprintf("<< /Title %s /Author %s /Subject %s%s /Creator (go-pdf) /Producer (go-pdf) /CreationDate (%s) >>",
		pdfString(d.opts.Title), pdfString(d.opts.Author), pdfString(d.opts.Subject), keywords, pdfDate(d.opts.CreationDate))
	w.set(infoID, []byte(info))

	// Deterministic document id: a hash of every object body.
	hash := sha256.New()
	for _, body := range w.bodies {
		if err := d.opts.Budget.Add(1); err != nil {
			return nil, err
		}
		hash.Write(body)
	}
	id := hex.EncodeToString(hash.Sum(nil))[:32]

	return assemble(w, catalogID, infoID, id)
}

// nominalWidth1000 returns a glyph width in thousandths of an em, rounded the
// same way for /W and for TJ adjustments.
func nominalWidth1000(f *font.Font, gid uint16) int32 {
	upem := float64(f.UnitsPerEm())
	if upem == 0 {
		return 0
	}
	return int32(math.Round(float64(f.Advance(gid)) * 1000 / upem))
}

func (fe *fontEntry) widthArray() string {
	// CIDs are contiguous from zero, so one group describes every width.
	widths := make([]string, len(fe.cidGID))
	for cid, gid := range fe.cidGID {
		widths[cid] = fmt.Sprintf("%d", nominalWidth1000(fe.font, gid))
	}
	if len(widths) == 0 {
		return ""
	}
	return fmt.Sprintf("0 [%s]", strings.Join(widths, " "))
}

func (fe *fontEntry) toUnicodeCMap() []byte {
	var cids []int
	for cid, r := range fe.cidRune {
		if r != 0 {
			cids = append(cids, cid)
		}
	}
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < len(cids); start += 100 {
		end := start + 100
		if end > len(cids) {
			end = len(cids)
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for _, cid := range cids[start:end] {
			fmt.Fprintf(&b, "<%04X> <%s>\n", cid, utf16Hex(fe.cidRune[cid]))
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return []byte(b.String())
}

// utf16Hex formats r as the big-endian UTF-16 code units of a PDF string. A
// BMP character is one four-digit group; a character outside the BMP becomes
// the surrogate pair unicode/utf16 defines.
func utf16Hex(r rune) string {
	var b strings.Builder
	for _, u := range utf16.Encode([]rune{r}) {
		fmt.Fprintf(&b, "%04X", u)
	}
	return b.String()
}

func (d *Doc) contentStream(p *page, fonts []*fontEntry, fontNames []string, imageNames map[*Image]string) []byte {
	nameOf := map[*font.Font]string{}
	byFont := map[*font.Font]*fontEntry{}
	for i, fe := range fonts {
		nameOf[fe.font] = fontNames[i]
		byFont[fe.font] = fe
	}
	var b strings.Builder
	for _, op := range p.ops {
		switch op.Kind {
		case FillOpKind:
			fmt.Fprintf(&b, "%s %s %s rg\n%s %s %s %s re f\n",
				f3b(op.Color.R), f3b(op.Color.G), f3b(op.Color.B),
				num(op.Rect.X), num(p.height-op.Rect.Y-op.Rect.H), num(op.Rect.W), num(op.Rect.H))
		case TextOpKind:
			if op.Text == nil || len(op.Text.Glyphs) == 0 {
				continue
			}
			fe := byFont[op.Text.Font]
			name := nameOf[op.Text.Font]
			fmt.Fprintf(&b, "BT /%s %s Tf %s %s %s rg 1 0 0 1 %s %s Tm [",
				name, num(op.Text.Size), f3b(op.Color.R), f3b(op.Color.G), f3b(op.Color.B),
				num(op.Text.X), num(p.height-op.Text.Y))
			for i, g := range op.Text.Glyphs {
				if i > 0 {
					b.WriteString(" ")
				}
				cid := uint16(0)
				if fe != nil {
					cid = fe.cidOf[pairKey(g.GID, g.Rune)]
				}
				fmt.Fprintf(&b, "<%04X>", cid)
				if i+1 < len(op.Text.Glyphs) {
					if adj := advanceAdjustment(op.Text.Font, op.Text.Size, g); adj != 0 {
						fmt.Fprintf(&b, " %d", adj)
					}
				}
			}
			b.WriteString("] TJ ET\n")
		case ImageOpKind:
			if op.Image == nil {
				continue
			}
			if clip := op.Clip; clip != nil && clip.W > 0 && clip.H > 0 {
				fmt.Fprintf(&b, "q %s %s %s %s re W n %s 0 0 %s %s %s cm /%s Do Q\n",
					num(clip.X), num(p.height-clip.Y-clip.H), num(clip.W), num(clip.H),
					num(op.Rect.W), num(op.Rect.H), num(op.Rect.X), num(p.height-op.Rect.Y-op.Rect.H),
					imageNames[op.Image])
				continue
			}
			fmt.Fprintf(&b, "q %s 0 0 %s %s %s cm /%s Do Q\n",
				num(op.Rect.W), num(op.Rect.H), num(op.Rect.X), num(p.height-op.Rect.Y-op.Rect.H), imageNames[op.Image])
		}
	}
	return []byte(b.String())
}

// advanceAdjustment is the TJ number that turns the nominal glyph width into
// the advance calculated by layout.
//
// A run with kerning between A and V looks like
//
//	[<0041> 42 <0056>] TJ
//
// where 42 moves V left by 42/1000 of the font size. A positive number always
// tightens the pair; a negative one loosens it.
func advanceAdjustment(f *font.Font, size int32, g Glyph) int32 {
	if f == nil || size <= 0 || g.Advance == 0 || f.UnitsPerEm() == 0 {
		return 0
	}
	// The nominal width comes from the same helper as the /W array, so the
	// viewer advance and this adjustment cannot disagree by a rounding unit.
	w1000 := float64(nominalWidth1000(f, g.GID))
	nominal := math.Round(w1000 * float64(size) / 1000)
	diff := nominal - float64(g.Advance)
	return int32(math.Round(diff * 1000 / float64(size)))
}

// streamObject wraps data in a stream object, optionally compressing it.
func streamObject(data []byte, compress bool) []byte {
	if compress {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(data); err == nil {
			_ = zw.Close()
			if buf.Len() < len(data) {
				return []byte(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", buf.Len(), buf.Bytes()))
			}
		}
	}
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

// imageObject returns the image XObject. alloc, when not nil, registers the
// complete image XObject of a PNG's alpha mask and returns its object id.
func imageObject(img *Image, alloc func([]byte) int) ([]byte, error) {
	switch img.Format {
	case "jpeg":
		// The frame header comes from the standard library, so the writer and
		// the engine agree on which files are valid.
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(img.Data))
		if err != nil {
			return nil, fmt.Errorf("pdf: decoding jpeg: %w", err)
		}
		var components int
		switch cfg.ColorModel {
		case color.GrayModel:
			components = 1
		case color.CMYKModel:
			components = 4
		default:
			components = 3
		}
		if components == 4 {
			// Go can decode CMYK JPEG, but the writer has no colour space or
			// inversion support for it; embedding the data as RGB would render
			// wrong colours.
			return nil, fmt.Errorf("pdf: CMYK JPEG images are not supported")
		}
		colorSpace := "/DeviceRGB"
		if components == 1 {
			colorSpace = "/DeviceGray"
		}
		return []byte(fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
			cfg.Width, cfg.Height, colorSpace, len(img.Data), img.Data)), nil
	case "png":
		w, h, rgb, alpha, err := pngPixels(img.Data)
		if err != nil {
			return nil, err
		}
		rgbData, err := deflate(rgb)
		if err != nil {
			return nil, err
		}
		if alpha == nil {
			return imageStream(w, h, "/DeviceRGB", rgbData, ""), nil
		}
		if alloc == nil {
			return nil, fmt.Errorf("pdf: a transparent png needs a soft mask object")
		}
		maskData, err := deflate(alpha)
		if err != nil {
			return nil, err
		}
		// The mask is an image XObject of its own: /SMask must point at a
		// complete image stream, not at raw pixel bytes, or validators and
		// viewers reject the image.
		maskID := alloc(imageStream(w, h, "/DeviceGray", maskData, ""))
		return imageStream(w, h, "/DeviceRGB", rgbData, fmt.Sprintf(" /SMask %d 0 R", maskID)), nil
	default:
		return nil, fmt.Errorf("pdf: unsupported image format %q", img.Format)
	}
}

// pngPixels decodes a PNG into raw RGB and, when any pixel is not opaque, an
// alpha channel. The colour channels are unpremultiplied so that a soft mask
// reproduces the source exactly over any page background.
func pngPixels(data []byte) (int, int, []byte, []byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("pdf: decoding png: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rgb := make([]byte, 0, w*h*3)
	alpha := make([]byte, 0, w*h)
	opaque := true
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			rgb = append(rgb, c.R, c.G, c.B)
			alpha = append(alpha, c.A)
			if c.A != 0xFF {
				opaque = false
			}
		}
	}
	if opaque {
		alpha = nil
	}
	return w, h, rgb, alpha, nil
}

func deflate(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func imageStream(w, h int, colorSpace string, data []byte, extra string) []byte {
	return []byte(fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /FlateDecode /Length %d%s >>\nstream\n%s\nendstream",
		w, h, colorSpace, len(data), extra, data))
}

func assemble(w *objectWriter, catalogID, infoID int, id string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(w.bodies)+1)
	for i, body := range w.bodies {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		buf.Write(body)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(w.bodies)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(w.bodies); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF\n",
		len(w.bodies)+1, catalogID, infoID, id, id, xref)
	return buf.Bytes(), nil
}

func num(mp int32) string { return f3(float64(mp) / 1000) }

func f3(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// f3b formats a colour component in the 0..1 range that PDF expects.
func f3b(v uint8) string { return f3(float64(v) / 255) }

func pdfDate(t time.Time) string {
	return t.UTC().Format("D:20060102150405Z")
}

func pdfString(s string) string {
	if s == "" {
		return "()"
	}
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
		if s[i] < 0x20 {
			ascii = false
		}
	}
	if !ascii {
		var b strings.Builder
		b.WriteString("<FEFF")
		for _, r := range s {
			b.WriteString(utf16Hex(r))
		}
		b.WriteString(">")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("(")
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', ')', '\\':
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteString(")")
	return b.String()
}

func sanitizeName(family, subfamily string) string {
	name := family
	if subfamily != "" && subfamily != "Regular" {
		name += "-" + subfamily
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "Font"
	}
	return b.String()
}
