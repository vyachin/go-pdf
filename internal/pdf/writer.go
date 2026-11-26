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
	"hash"
	"image"
	"image/color"
	"image/jpeg"
	"io"
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
	// Decoded is the image the loader already decoded, when it has one. The
	// writer reuses it instead of decoding the same PNG a second time.
	Decoded image.Image
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
	// base is the id before the first object. It lets the outline be built in
	// the streaming writer's id space without pretending its ids start at one.
	base int
}

func (w *objectWriter) add() int {
	w.bodies = append(w.bodies, nil)
	return w.base + len(w.bodies)
}

func (w *objectWriter) set(id int, body []byte) {
	w.bodies[id-1-w.base] = body
}

// streamWriter writes objects straight to an io.Writer, records their offsets
// for the cross-reference table and hashes their bodies for the document
// identifier. It never holds more than the object it is writing.
type streamWriter struct {
	w       io.Writer
	guard   *budget.Guard
	written int64
	nextID  int
	offsets []int64
	hash    hash.Hash
}

func newStreamWriter(w io.Writer, guard *budget.Guard, firstID int) *streamWriter {
	return &streamWriter{
		w:       w,
		guard:   guard,
		nextID:  firstID,
		offsets: make([]int64, firstID-1),
		hash:    sha256.New(),
	}
}

// add reserves the next object id. Its offset is recorded when it is emitted.
func (s *streamWriter) add() int {
	id := s.nextID
	s.nextID++
	s.offsets = append(s.offsets, 0)
	return id
}

// charge records the work of writing n bytes, stopping an exhausted budget or
// a cancelled context before more of the object is written.
func (s *streamWriter) charge(n int) error {
	return s.guard.Add(uint64(n/4096) + 1)
}

// write copies p to the destination and records the bytes it accepted, even
// when the write fails part way through.
func (s *streamWriter) write(p []byte) error {
	n, err := writeAll(s.w, p)
	s.written += int64(n)
	return err
}

func (s *streamWriter) writeString(text string) error {
	return s.write([]byte(text))
}

// writeHashed writes object body bytes and adds them to the content hash.
func (s *streamWriter) writeHashed(p []byte) error {
	if _, err := s.hash.Write(p); err != nil {
		return err
	}
	return s.write(p)
}

// beginObject records the offset of id and writes its header.
func (s *streamWriter) beginObject(id int) error {
	if id < 1 || id > len(s.offsets) {
		return fmt.Errorf("pdf: invalid object id %d", id)
	}
	if s.offsets[id-1] != 0 {
		return fmt.Errorf("pdf: object %d was written twice", id)
	}
	s.offsets[id-1] = s.written
	return s.writeString(fmt.Sprintf("%d 0 obj\n", id))
}

func (s *streamWriter) endObject() error {
	return s.writeString("\nendobj\n")
}

// emit writes one complete object body.
func (s *streamWriter) emit(id int, body []byte) error {
	if err := s.charge(len(body)); err != nil {
		return err
	}
	if err := s.beginObject(id); err != nil {
		return err
	}
	if err := s.writeHashed(body); err != nil {
		return err
	}
	return s.endObject()
}

// emitStream writes a stream object, compressing the data when that makes it
// smaller. The compressed bytes are written directly, so no second copy of the
// complete object is built and only the data of the current object is live.
func (s *streamWriter) emitStream(id int, data []byte) error {
	if err := s.charge(len(data)); err != nil {
		return err
	}
	wire, compressed := compressedBody(data, newZlibWriter)
	filter := ""
	if compressed {
		filter = " /Filter /FlateDecode"
	}
	if err := s.beginObject(id); err != nil {
		return err
	}
	dict := fmt.Sprintf("<< /Length %d%s >>\nstream\n", len(wire), filter)
	if err := s.writeHashed([]byte(dict)); err != nil {
		return err
	}
	if err := s.writeHashed(wire); err != nil {
		return err
	}
	if err := s.writeHashed([]byte("\nendstream")); err != nil {
		return err
	}
	return s.endObject()
}

// writeXref writes the cross-reference table and the trailer. It is the last
// step, so every offset is known; a cancellation before it is reported instead
// of leaving a document that points at a partial table.
func (s *streamWriter) writeXref(catalogID, infoID int, id string) error {
	if err := s.guard.Err(); err != nil {
		return err
	}
	for i, off := range s.offsets {
		if off == 0 {
			return fmt.Errorf("pdf: object %d was not written", i+1)
		}
	}
	xref := s.written
	var b strings.Builder
	fmt.Fprintf(&b, "xref\n0 %d\n", len(s.offsets)+1)
	b.WriteString("0000000000 65535 f \n")
	for _, off := range s.offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF\n",
		len(s.offsets)+1, catalogID, infoID, id, id, xref)
	return s.writeString(b.String())
}

// writeAll writes p completely and reports the bytes accepted. A writer that
// reports success without accepting a byte can never complete the write, so it
// is reported as io.ErrShortWrite instead of being retried forever.
func writeAll(w io.Writer, p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n, err := w.Write(p)
		if n < 0 || n > len(p) {
			return written, fmt.Errorf("pdf: writer returned an invalid count %d", n)
		}
		written += n
		p = p[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
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

// Bytes serializes the document. It is [Doc.WriteTo] into a buffer, so the
// buffered and the streaming entry points share one serialiser and cannot
// drift apart.
func (d *Doc) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pdfHeader is the file header every document starts with.
const pdfHeader = "%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"

// WriteTo serializes the document to w and returns the number of bytes
// written.
//
// The document is written in the streaming order: the header, each page's
// content stream and page object, the fonts and images discovered on those
// pages, the outline, the page tree, the catalog, the info dictionary, the
// cross-reference table and the trailer. Object identifiers are assigned
// before the first write, and every offset is recorded from a counting writer,
// so serialisation holds at most one page's content stream rather than the
// complete PDF.
//
// A write error is returned as w reported it: a short write becomes
// [io.ErrShortWrite] and no failure path panics. The context carried by
// Options.Budget is checked between pages and before the cross-reference
// table, so a cancelled conversion stops at a page boundary and returns the
// context error. A failed call may already have written a prefix of the
// document to w; the caller must discard it.
func (d *Doc) WriteTo(w io.Writer) (int64, error) {
	if w == nil {
		return 0, fmt.Errorf("pdf: nil writer")
	}
	if err := d.opts.Budget.Err(); err != nil {
		return 0, err
	}
	if len(d.pages) == 0 {
		return 0, fmt.Errorf("pdf: document has no pages")
	}

	fonts, images, err := d.collectObjects()
	if err != nil {
		return 0, err
	}

	// The page count is known before the first byte, so the page objects are
	// reserved first and can be written while their content is produced. Fonts
	// and images become known from the operations of those pages; their bodies
	// follow the last page.
	nextID := 1
	contentIDs := make([]int, len(d.pages))
	pageIDs := make([]int, len(d.pages))
	for i := range d.pages {
		contentIDs[i] = nextID
		nextID++
		pageIDs[i] = nextID
		nextID++
	}
	for _, fe := range fonts {
		// Type0, CIDFontType2, FontDescriptor, FontFile2 and ToUnicode each
		// need their own object: reusing one id for two roles breaks the chain.
		fe.ids = make([]int, 6)
		for i := range fe.ids {
			fe.ids[i] = nextID
			nextID++
		}
	}
	for _, im := range images {
		im.id = nextID
		nextID++
	}
	pagesID := nextID
	nextID++
	catalogID := nextID
	nextID++
	infoID := nextID
	nextID++

	// The outline ids are known before the first page and their bodies are
	// held until the document-level objects are written; an outline is small
	// and does not grow with the number of pages.
	var outline *objectWriter
	outlineRoot := 0
	if len(d.opts.Outline) > 0 {
		outline = &objectWriter{base: nextID - 1}
		outlineRoot = outline.add()
		writeOutline(outline, d.opts.Outline, outlineRoot, d.pages, pageIDs, outlineRoot)
		nextID += len(outline.bodies)
	}

	fontNames := make([]string, len(fonts))
	for i := range fonts {
		fontNames[i] = fmt.Sprintf("F%d", i+1)
	}
	imageNames := map[*Image]string{}
	for i, im := range images {
		imageNames[im.image] = fmt.Sprintf("Im%d", i+1)
	}

	// Page content addresses CIDs, so the CID map is built before the first
	// page is written. The subset itself is built when its object is emitted,
	// after the pages.
	for _, fe := range fonts {
		// Subsetting, the CID map, /W and the ToUnicode map all grow with the
		// glyphs and the distinct glyph-rune pairs of the font.
		if err := d.opts.Budget.Add(uint64(len(fe.used) + len(fe.pairs) + 1)); err != nil {
			return 0, err
		}
		if len(fe.pairs) > 0xFFFF {
			return 0, fmt.Errorf("%w: one font needs %d distinct glyph-rune pairs, the limit is 65535", budget.ErrExceeded, len(fe.pairs))
		}
		fe.buildCIDs()
	}

	sw := newStreamWriter(w, d.opts.Budget, nextID)
	if err := sw.writeString(pdfHeader); err != nil {
		return sw.written, err
	}

	// Page content and page objects.
	for i, p := range d.pages {
		// A cancelled context or an exhausted budget stops at a page boundary
		// instead of after the whole document.
		if err := d.opts.Budget.Err(); err != nil {
			return sw.written, err
		}
		if err := d.opts.Budget.Add(uint64(len(p.ops)) + 1); err != nil {
			return sw.written, err
		}
		content, err := d.contentStream(p, fonts, fontNames, imageNames)
		if err != nil {
			return sw.written, err
		}
		if err := sw.emitStream(contentIDs[i], content); err != nil {
			return sw.written, err
		}
		var fontRes, imageRes strings.Builder
		for fi := range fonts {
			fmt.Fprintf(&fontRes, " /%s %d 0 R", fontNames[fi], fonts[fi].ids[0])
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
		if imageRes.Len() > 0 {
			resources += fmt.Sprintf(" /XObject <<%s >>", imageRes.String())
		}
		pageBody := fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << %s >> /Contents %d 0 R >>",
			pagesID, num(p.width), num(p.height), resources, contentIDs[i])
		if err := sw.emit(pageIDs[i], []byte(pageBody)); err != nil {
			return sw.written, err
		}
	}

	// Font objects.
	for _, fe := range fonts {
		if err := d.opts.Budget.Err(); err != nil {
			return sw.written, err
		}
		if err := d.opts.Budget.Add(1); err != nil {
			return sw.written, err
		}
		subset, err := fe.font.Subset(fe.used)
		if err != nil {
			return sw.written, fmt.Errorf("pdf: subsetting font: %w", err)
		}
		fe.subset = subset
		fontID, cidID := fe.ids[0], fe.ids[1]
		descID, fileID, toUniID := fe.ids[2], fe.ids[3], fe.ids[4]
		cidMapID := fe.ids[5]
		baseName := baseFontName(subsetTag(fe.subset), fe.font.Family(), fe.font.Subfamily())
		upem := float64(fe.font.UnitsPerEm())
		scale := 1000 / upem
		bbox := fe.font.BBox()
		widths, err := fe.widthArray(d.opts.Budget)
		if err != nil {
			return sw.written, err
		}
		toUni, err := fe.toUnicodeCMap(d.opts.Budget)
		if err != nil {
			return sw.written, err
		}
		// The descriptor's /Flags and /ItalicAngle must agree: a consumer that
		// reads either one has to reach the same conclusion about the face.
		italicAngle := fe.font.ItalicAngle()
		flags := 4
		if italicAngle != 0 {
			flags |= 64
		}

		bodies := []struct {
			id   int
			body []byte
		}{
			{fontID, []byte(fmt.Sprintf(
				"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
				baseName, cidID, toUniID))},
			{cidID, []byte(fmt.Sprintf(
				"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /DW 1000 /W [%s] /CIDToGIDMap %d 0 R >>",
				baseName, descID, widths, cidMapID))},
			{descID, []byte(fmt.Sprintf(
				"<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%d %d %d %d] /ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R >>",
				baseName, flags,
				int32(float64(bbox[0])*scale), int32(float64(bbox[1])*scale),
				int32(float64(bbox[2])*scale), int32(float64(bbox[3])*scale),
				numFloat(italicAngle),
				int32(float64(fe.font.Ascent())*scale),
				int32(float64(fe.font.Descent())*scale),
				int32(float64(fe.font.CapHeight())*scale),
				fileID))},
			{fileID, streamObject(fe.subset, true)},
			{toUniID, streamObject(toUni, true)},
			{cidMapID, streamObject(fe.cidToGIDMap(), true)},
		}
		for _, object := range bodies {
			if err := sw.emit(object.id, object.body); err != nil {
				return sw.written, err
			}
		}
	}

	// Image objects. A PNG with transparency needs a second stream for its
	// soft mask; the allocator reserves its object id and the mask body is
	// written right after the image that references it.
	for _, im := range images {
		if err := d.opts.Budget.Err(); err != nil {
			return sw.written, err
		}
		type maskObject struct {
			id   int
			body []byte
		}
		var masks []maskObject
		body, err := imageObject(d.opts.Budget, im.image, func(mask []byte) int {
			id := sw.add()
			masks = append(masks, maskObject{id: id, body: mask})
			return id
		})
		if err != nil {
			return sw.written, err
		}
		if err := sw.emit(im.id, body); err != nil {
			return sw.written, err
		}
		for _, mask := range masks {
			if err := sw.emit(mask.id, mask.body); err != nil {
				return sw.written, err
			}
		}
	}

	// Outline, page tree, catalog and info dictionary.
	if outline != nil {
		for i, body := range outline.bodies {
			if err := sw.emit(outline.base+1+i, body); err != nil {
				return sw.written, err
			}
		}
	}
	kids := make([]string, len(pageIDs))
	for i, id := range pageIDs {
		kids[i] = fmt.Sprintf("%d 0 R", id)
	}
	if err := sw.emit(pagesID, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageIDs)))); err != nil {
		return sw.written, err
	}
	if outlineRoot != 0 {
		if err := sw.emit(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R /PageMode /UseOutlines >>", pagesID, outlineRoot))); err != nil {
			return sw.written, err
		}
	} else {
		if err := sw.emit(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))); err != nil {
			return sw.written, err
		}
	}

	keywords := ""
	if d.opts.Keywords != "" {
		keywords = " /Keywords " + pdfString(d.opts.Keywords)
	}
	info := fmt.Sprintf("<< /Title %s /Author %s /Subject %s%s /Creator (go-pdf) /Producer (go-pdf) /CreationDate (%s) >>",
		pdfString(d.opts.Title), pdfString(d.opts.Author), pdfString(d.opts.Subject), keywords, pdfDate(d.opts.CreationDate))
	if err := sw.emit(infoID, []byte(info)); err != nil {
		return sw.written, err
	}

	// Deterministic document id: a hash of every object body in the order it
	// was written.
	id := hex.EncodeToString(sw.hash.Sum(nil))[:32]
	if err := sw.writeXref(catalogID, infoID, id); err != nil {
		return sw.written, err
	}
	return sw.written, nil
}

// collectObjects gathers the fonts and images in first-use order, which keeps
// the object numbering and the output deterministic. Both passes charge the
// work guard, as they inspect every operation of every page.
func (d *Doc) collectObjects() ([]*fontEntry, []*imageEntry, error) {
	var fonts []*fontEntry
	fontByPtr := map[*font.Font]*fontEntry{}
	for _, p := range d.pages {
		if err := d.opts.Budget.Add(uint64(len(p.ops)) + 1); err != nil {
			return nil, nil, err
		}
		for _, op := range p.ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			if op.Text.Font == nil {
				return nil, nil, fmt.Errorf("pdf: a text operation has no font")
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
			return nil, nil, err
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
	return fonts, images, nil
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

// maxArrayElements is the compatibility limit for one array in a content
// stream or a font dictionary. Viewers accept it and a larger array can trip
// their parser.
const maxArrayElements = 8191

func (fe *fontEntry) widthArray(guard *budget.Guard) (string, error) {
	widths := make([]string, len(fe.cidGID))
	for cid, gid := range fe.cidGID {
		widths[cid] = fmt.Sprintf("%d", nominalWidth1000(fe.font, gid))
	}
	if err := guard.Add(uint64(len(widths)) + 1); err != nil {
		return "", err
	}
	if len(widths) == 0 {
		return "", nil
	}
	// CIDs are contiguous from zero. A font subset can carry thousands of
	// distinct widths, so /W is written as several startCID groups, none
	// larger than the compatibility limit.
	var groups []string
	for start := 0; start < len(widths); start += maxArrayElements {
		end := start + maxArrayElements
		if end > len(widths) {
			end = len(widths)
		}
		groups = append(groups, fmt.Sprintf("%d [%s]", start, strings.Join(widths[start:end], " ")))
	}
	if err := guard.Add(uint64(len(groups))); err != nil {
		return "", err
	}
	return strings.Join(groups, " "), nil
}

func (fe *fontEntry) toUnicodeCMap(guard *budget.Guard) ([]byte, error) {
	// Every emitted CID gets an entry. A glyph whose source rune is U+0000 is
	// mapped to <0000> rather than skipped, so the text layer covers the whole
	// run and the entry count matches the number of emitted CIDs.
	count := len(fe.cidRune)
	// One unit per entry in the map before it is written.
	if err := guard.Add(uint64(count) + 1); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < count; start += 100 {
		end := start + 100
		if end > count {
			end = count
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for cid := start; cid < end; cid++ {
			fmt.Fprintf(&b, "<%04X> <%s>\n", cid, utf16Hex(fe.cidRune[cid]))
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return []byte(b.String()), nil
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

func (d *Doc) contentStream(p *page, fonts []*fontEntry, fontNames []string, imageNames map[*Image]string) ([]byte, error) {
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
			// Every serialised glyph is charged before it is written.
			if err := d.opts.Budget.Add(uint64(len(op.Text.Glyphs))); err != nil {
				return nil, err
			}
			fe := byFont[op.Text.Font]
			name := nameOf[op.Text.Font]
			fmt.Fprintf(&b, "BT /%s %s Tf %s %s %s rg 1 0 0 1 %s %s Tm\n",
				name, num(op.Text.Size), f3b(op.Color.R), f3b(op.Color.G), f3b(op.Color.B),
				num(op.Text.X), num(p.height-op.Text.Y))
			// Contiguous glyphs share one hexadecimal string. A string holds
			// at most maxArrayElements four-digit CIDs (32,764 bytes, under the
			// 32,767-byte limit), so a longer run is split across several TJ
			// operators without moving a glyph.
			var parts []string
			var hex strings.Builder
			flushString := func() {
				if hex.Len() > 0 {
					parts = append(parts, "<"+hex.String()+">")
					hex.Reset()
				}
			}
			for i, g := range op.Text.Glyphs {
				if hex.Len() >= 4*maxArrayElements {
					flushString()
				}
				cid := uint16(0)
				if fe != nil {
					cid = fe.cidOf[pairKey(g.GID, g.Rune)]
				}
				fmt.Fprintf(&hex, "%04X", cid)
				if i+1 < len(op.Text.Glyphs) {
					if adj := advanceAdjustment(op.Text.Font, op.Text.Size, g); adj != 0 {
						flushString()
						parts = append(parts, strconv.Itoa(int(adj)))
					}
				}
			}
			flushString()
			// Each TJ operator carries at most maxArrayElements glyph codes, so
			// a long run becomes several bounded arrays. All operators stay in
			// one text object, so the glyph positions are unchanged.
			var chunk []string
			chunkCIDs := 0
			flushChunk := func() {
				if len(chunk) == 0 {
					return
				}
				fmt.Fprintf(&b, "[%s] TJ\n", strings.Join(chunk, " "))
				chunk, chunkCIDs = nil, 0
			}
			for _, part := range parts {
				if len(chunk) >= maxArrayElements || chunkCIDs >= maxArrayElements {
					flushChunk()
				}
				chunk = append(chunk, part)
				chunkCIDs += cidCount(part)
			}
			flushChunk()
			b.WriteString("ET\n")
		case ImageOpKind:
			if op.Image == nil {
				continue
			}
			// A zero-area image would emit a non-invertible transformation
			// matrix; the operation is omitted instead.
			if op.Rect.W <= 0 || op.Rect.H <= 0 {
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
	return []byte(b.String()), nil
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

// compressorFunc creates a compressor stream. It is a parameter of the
// package-private streamObjectWith so a test can inject a failing writer without
// a mutable global on the conversion path.
type compressorFunc func(io.Writer) io.WriteCloser

// newZlibWriter is the compressor every stream uses in production.
func newZlibWriter(w io.Writer) io.WriteCloser { return zlib.NewWriter(w) }

// compressedBody returns the bytes to embed and whether they are Flate
// compressed. The compressor is closed exactly once on every path and its error
// is handled: a write or close failure, or a result that does not shrink the
// data, falls back to the original bytes. Compression is a size optimisation,
// so a failure must not truncate a stream or lose the object.
func compressedBody(data []byte, newWriter compressorFunc) (wire []byte, compressed bool) {
	var buf bytes.Buffer
	zw := newWriter(&buf)
	_, writeErr := zw.Write(data)
	closeErr := zw.Close()
	if writeErr != nil || closeErr != nil || buf.Len() >= len(data) {
		return data, false
	}
	return buf.Bytes(), true
}

// streamObject wraps data in a stream object, optionally compressing it.
func streamObject(data []byte, compress bool) []byte {
	return streamObjectWith(data, compress, newZlibWriter)
}

// streamObjectWith is streamObject with the compressor injectable for tests.
func streamObjectWith(data []byte, compress bool, newWriter compressorFunc) []byte {
	wire, filter := data, ""
	if compress {
		if compressed, ok := compressedBody(data, newWriter); ok {
			wire, filter = compressed, " /Filter /FlateDecode"
		}
	}
	return []byte(fmt.Sprintf("<< /Length %d%s >>\nstream\n%s\nendstream", len(wire), filter, wire))
}

// imageObject returns the image XObject. alloc, when not nil, registers the
// complete image XObject of a PNG's alpha mask and returns its object id.
func imageObject(guard *budget.Guard, img *Image, alloc func([]byte) int) ([]byte, error) {
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
		decoded := img.Decoded
		var header pngHeader
		var err error
		if decoded != nil {
			header, err = readPNGHeader(img.Data)
		} else {
			decoded, header, err = decodedPNG(img.Data, guard)
		}
		if err != nil {
			return nil, err
		}
		samples, err := pngToSamples(header, decoded, guard)
		if err != nil {
			return nil, err
		}
		colorData, err := deflate(samples.color)
		if err != nil {
			return nil, err
		}
		if samples.alpha == nil {
			return imageStream(samples.width, samples.height, samples.colorSpace, samples.bits, colorData, ""), nil
		}
		if alloc == nil {
			return nil, fmt.Errorf("pdf: a transparent png needs a soft mask object")
		}
		maskData, err := deflate(samples.alpha)
		if err != nil {
			return nil, err
		}
		// The mask is an image XObject of its own: /SMask must point at a
		// complete image stream, not at raw pixel bytes, or validators and
		// viewers reject the image.
		maskID := alloc(imageStream(samples.width, samples.height, "/DeviceGray", 8, maskData, ""))
		return imageStream(samples.width, samples.height, samples.colorSpace, samples.bits, colorData, fmt.Sprintf(" /SMask %d 0 R", maskID)), nil
	default:
		return nil, fmt.Errorf("pdf: unsupported image format %q", img.Format)
	}
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

func imageStream(w, h int, colorSpace string, bits int, data []byte, extra string) []byte {
	return []byte(fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent %d /Filter /FlateDecode /Length %d%s >>\nstream\n%s\nendstream",
		w, h, colorSpace, bits, len(data), extra, data))
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
	u := t.UTC()
	// Defensive: the engine validates the UTC year before it writes, but the
	// writer must never emit a five-digit year even when a caller reaches it
	// directly.
	if y := u.Year(); y < 0 || y > 9999 {
		u = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	return u.Format("D:20060102150405Z")
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

// maxBaseFontBytes is the architectural limit of a name in a PDF dictionary.
const maxBaseFontBytes = 127

// baseFontName builds the /BaseFont and /FontName of one subset: the six-letter
// tag, a plus and the sanitized family. A family name long enough to cross the
// 127-byte limit is truncated, so the name stays legal while every subset keeps
// its own tag.
func baseFontName(tag, family, subfamily string) string {
	name := sanitizeName(family, subfamily)
	room := maxBaseFontBytes - len(tag) - 1
	if room < 1 {
		room = 1
	}
	if len(name) > room {
		name = strings.TrimRight(name[:room], "-")
		if name == "" {
			name = "Font"
		}
	}
	return tag + "+" + name
}

// cidCount returns the number of glyph codes a TJ element carries: a
// hexadecimal string holds one code per four digits, an adjustment none.
func cidCount(part string) int {
	if len(part) < 2 || part[0] != '<' {
		return 0
	}
	return (len(part) - 2) / 4
}
