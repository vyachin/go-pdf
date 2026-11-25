// Package pdfdebug reads back the text of PDF files produced by this engine.
//
// It is a test and debugging helper, not a general PDF parser: it relies on the
// writer's structure (one content stream per page, Identity-H fonts with a
// ToUnicode map) and is used to assert that Russian text survives into the
// output and to build regression goldens.
package pdfdebug

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

var (
	reKids        = regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`)
	reContentsArr = regexp.MustCompile(`/Contents\s*\[([^\]]*)\]`)
	reContentsRef = regexp.MustCompile(`/Contents\s+(\d+)\s+0\s+R`)
	reRef         = regexp.MustCompile(`(\d+)\s+0\s+R`)
)

// Pages returns the extracted text of every page, in page-tree order.
func Pages(data []byte) ([]string, error) {
	if err := Validate(data); err != nil {
		// A structurally corrupt file must not produce the same text as a valid
		// one: the extractor would otherwise certify the writer's mistake.
		return nil, err
	}
	objects := splitObjects(data)
	toUniByID := buildToUniByID(objects)
	fontMaps := contentFontMaps(objects, toUniByID)
	ids := pageContentIDs(objects)
	if len(ids) == 0 {
		// A document without a readable page tree: fall back to the content
		// streams in object order, which is the pre-page-tree behaviour.
		for id := range objects {
			ids = append(ids, id)
		}
		slices.Sort(ids)
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		obj := objects[id]
		if obj == nil {
			continue
		}
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
		out = append(out, textFromStream(stream, fontMaps[id]))
	}
	return out, nil
}

// ContentStreams returns the decoded content streams of the document in page
// order. It is used by tests that assert on emitted operators rather than on
// text.
func ContentStreams(data []byte) [][]byte {
	objects := splitObjects(data)
	ids := pageContentIDs(objects)
	if len(ids) == 0 {
		for id := range objects {
			ids = append(ids, id)
		}
		slices.Sort(ids)
	}
	var out [][]byte
	for _, id := range ids {
		obj := objects[id]
		if obj == nil {
			continue
		}
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
		out = append(out, stream)
	}
	return out
}

// pageContentIDs walks the page tree from its root and returns each page's
// /Contents object id in page order. A page's /Contents can be a single
// reference or an array of references.
func pageContentIDs(objects map[int]*pdfObject) []int {
	root := -1
	for id, obj := range objects {
		if bytes.Contains(obj.body, []byte("/Type /Pages")) && !bytes.Contains(obj.body, []byte("/Parent")) {
			if root < 0 || id < root {
				root = id
			}
		}
	}
	var out []int
	if root >= 0 {
		seen := map[int]bool{}
		var walk func(int)
		walk = func(id int) {
			if seen[id] {
				return
			}
			seen[id] = true
			obj := objects[id]
			if obj == nil {
				return
			}
			if bytes.Contains(obj.body, []byte("/Type /Page")) && !bytes.Contains(obj.body, []byte("/Type /Pages")) {
				out = append(out, contentRefs(obj.body)...)
				return
			}
			m := reKids.FindSubmatch(obj.body)
			if m == nil {
				return
			}
			for _, ref := range reRef.FindAllSubmatch(m[1], -1) {
				n, _ := strconv.Atoi(string(ref[1]))
				walk(n)
			}
		}
		walk(root)
	}
	if len(out) > 0 {
		return out
	}
	// No usable tree: every page object, in object order.
	var pages []int
	for id, obj := range objects {
		if bytes.Contains(obj.body, []byte("/Type /Page")) && !bytes.Contains(obj.body, []byte("/Type /Pages")) {
			pages = append(pages, id)
		}
	}
	slices.Sort(pages)
	for _, id := range pages {
		out = append(out, contentRefs(objects[id].body)...)
	}
	return out
}

func contentRefs(body []byte) []int {
	if m := reContentsArr.FindSubmatch(body); m != nil {
		var ids []int
		for _, ref := range reRef.FindAllSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(string(ref[1]))
			ids = append(ids, n)
		}
		return ids
	}
	if m := reContentsRef.FindSubmatch(body); m != nil {
		n, _ := strconv.Atoi(string(m[1]))
		return []int{n}
	}
	return nil
}

// Text returns the text of the whole document with form feed separators
// between pages, similar to pdftotext.
func Text(data []byte) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	pages, err := Pages(data)
	if err != nil {
		return "", err
	}
	return strings.Join(pages, "\f\n"), nil
}

type pdfObject struct {
	body []byte
}

func splitObjects(data []byte) map[int]*pdfObject {
	objects := map[int]*pdfObject{}
	rest := data
	for {
		idx := bytes.Index(rest, []byte(" obj\n"))
		if idx < 0 {
			break
		}
		lineStart := bytes.LastIndexByte(rest[:idx], '\n') + 1
		fields := strings.Fields(string(rest[lineStart:idx]))
		if len(fields) < 1 {
			rest = rest[idx+len(" obj\n"):]
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			rest = rest[idx+len(" obj\n"):]
			continue
		}
		body := rest[idx+len(" obj\n"):]
		endIdx := bytes.Index(body, []byte("\nendobj"))
		if endIdx < 0 {
			break
		}
		objects[id] = &pdfObject{body: body[:endIdx]}
		rest = body[endIdx+len("\nendobj"):]
	}
	return objects
}

func decodedStream(body []byte) ([]byte, error) {
	idx := bytes.Index(body, []byte("stream\n"))
	if idx < 0 {
		return nil, fmt.Errorf("no stream")
	}
	start := idx + len("stream\n")
	end := bytes.Index(body[start:], []byte("\nendstream"))
	if end < 0 {
		return nil, fmt.Errorf("no endstream")
	}
	raw := body[start : start+end]
	if !bytes.Contains(body[:idx], []byte("FlateDecode")) {
		return raw, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildToUniByID maps each font object to its own CID table, so the same code
// in two different fonts does not resolve to the same character.
func buildToUniByID(objects map[int]*pdfObject) map[int]map[uint16]rune {
	out := map[int]map[uint16]rune{}
	for id, obj := range objects {
		idx := bytes.Index(obj.body, []byte("/ToUnicode"))
		if idx < 0 {
			continue
		}
		ref := parseIntToken(obj.body[idx+len("/ToUnicode"):])
		target, ok := objects[ref]
		if !ok {
			continue
		}
		stream, err := decodedStream(target.body)
		if err != nil {
			continue
		}
		table := map[uint16]rune{}
		parseBFChar(stream, table)
		out[id] = table
	}
	return out
}

// contentFontMaps links each content stream to the font tables of its page,
// keyed by the resource name used in "Tf" operators.
func contentFontMaps(objects map[int]*pdfObject, toUniByID map[int]map[uint16]rune) map[int]map[string]map[uint16]rune {
	out := map[int]map[string]map[uint16]rune{}
	for _, page := range objects {
		if !bytes.Contains(page.body, []byte("/Type /Page ")) {
			continue
		}
		contentID := -1
		if i := bytes.Index(page.body, []byte("/Contents")); i >= 0 {
			contentID = parseIntToken(page.body[i+len("/Contents"):])
		}
		if contentID < 0 {
			continue
		}
		names := map[string]map[uint16]rune{}
		if i := bytes.Index(page.body, []byte("/Font")); i >= 0 {
			rest := page.body[i:]
			if open := bytes.Index(rest, []byte("<<")); open >= 0 {
				if closeIdx := bytes.Index(rest[open:], []byte(">>")); closeIdx > 0 {
					fields := bytes.Fields(rest[open+2 : open+closeIdx])
					for j := 0; j+1 < len(fields); j++ {
						if fields[j][0] != '/' {
							continue
						}
						id, err := strconv.Atoi(string(fields[j+1]))
						if err != nil {
							continue
						}
						if table := toUniByID[id]; table != nil {
							names[string(fields[j][1:])] = table
						}
					}
				}
			}
		}
		out[contentID] = names
	}
	return out
}

func parseBFChar(stream []byte, out map[uint16]rune) {
	for _, line := range bytes.Split(stream, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) < 10 || line[0] != '<' {
			continue
		}
		endA := bytes.IndexByte(line, '>')
		if endA < 0 {
			continue
		}
		hexA := string(line[1:endA])
		rest := bytes.TrimSpace(line[endA+1:])
		if len(rest) < 3 || rest[0] != '<' {
			continue
		}
		endB := bytes.IndexByte(rest, '>')
		if endB < 0 {
			continue
		}
		hexB := string(rest[1:endB])
		gidBytes, err := hex.DecodeString(hexA)
		if err != nil || len(gidBytes) != 2 {
			continue
		}
		gid := uint16(gidBytes[0])<<8 | uint16(gidBytes[1])
		if runes := decodeUTF16Hex(hexB); len(runes) > 0 {
			out[gid] = runes[0]
		}
	}
}

// decodeUTF16Hex decodes the hex string of a ToUnicode destination. A surrogate
// pair is one code point, not two: the same text a reader copies out of the
// PDF.
func decodeUTF16Hex(s string) []rune {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw)%2 != 0 {
		return nil
	}
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		units = append(units, uint16(raw[i])<<8|uint16(raw[i+1]))
	}
	return utf16.Decode(units)
}

// textFromStream reads both plain Tj strings and TJ arrays. Adjustments in a
// TJ array only move glyphs; extraction ignores them.
func textFromStream(stream []byte, fontMaps map[string]map[uint16]rune) string {
	var out strings.Builder
	s := string(stream)
	inArray := false
	current := map[uint16]rune{}
	for i := 0; i < len(s); {
		switch {
		case s[i] == '[':
			inArray = true
			i++
		case s[i] == ']':
			inArray = false
			i++
		case s[i] == 'T' && strings.HasPrefix(s[i:], "Tf"):
			// Switch to the table of the font named by the preceding /Name.
			if slash := strings.LastIndex(s[:i], "/"); slash >= 0 {
				name := s[slash+1:]
				if cut := strings.IndexAny(name, " \n\r\t"); cut >= 0 {
					name = name[:cut]
				}
				if table, ok := fontMaps[name]; ok {
					current = table
				} else {
					current = map[uint16]rune{}
				}
			}
			i += 2
		case s[i] == '<':
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				return out.String()
			}
			hexStr := s[i+1 : i+end]
			i += end + 1
			if inArray {
				out.WriteString(decodeHexText(hexStr, current))
				continue
			}
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\r' || s[j] == '\t') {
				j++
			}
			if strings.HasPrefix(s[j:], "Tj") {
				out.WriteString(decodeHexText(hexStr, current))
				i = j + 2
			}
		default:
			i++
		}
	}
	return out.String()
}

func decodeHexText(hexStr string, toUni map[uint16]rune) string {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		return ""
	}
	var out strings.Builder
	for i := 0; i+1 < len(raw); i += 2 {
		gid := uint16(raw[i])<<8 | uint16(raw[i+1])
		if r, ok := toUni[gid]; ok {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func parseIntToken(b []byte) int {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n') {
		i++
	}
	start := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	id, _ := strconv.Atoi(string(b[start:i]))
	return id
}

// PageCount reports the number of pages in the document, walked from the page
// tree. Unlike Pages it does not depend on a page drawing text, so a page that
// carries only an image, or nothing at all, still counts.
func PageCount(data []byte) (int, error) {
	refs, err := pageRefs(data)
	if err != nil {
		return 0, err
	}
	return len(refs), nil
}

// PageTexts returns the text of every page in page-tree order, one entry per
// page. A page whose content stream draws no text yields an empty string;
// Pages, which predates this function, drops such a page instead.
func PageTexts(data []byte) ([]string, error) {
	if err := Validate(data); err != nil {
		return nil, err
	}
	refs, err := pageRefs(data)
	if err != nil {
		return nil, err
	}
	objects := splitObjects(data)
	fontMaps := contentFontMaps(objects, buildToUniByID(objects))
	texts := make([]string, len(refs))
	for i, pageID := range refs {
		page, ok := objects[pageID]
		if !ok {
			continue
		}
		contentID := contentRef(page.body)
		content, ok := objects[contentID]
		if !ok {
			continue
		}
		stream, err := decodedStream(content.body)
		if err != nil {
			continue
		}
		texts[i] = textFromStream(stream, fontMaps[contentID])
	}
	return texts, nil
}

// pageRefs returns the object id of every page, in page-tree order. It walks
// /Kids from the root /Pages node so that a nested tree keeps its order.
func pageRefs(data []byte) ([]int, error) {
	objects := splitObjects(data)
	root := -1
	for id, obj := range objects {
		if isObjectType(obj.body, "Pages") && !bytes.Contains(obj.body, []byte("/Parent")) {
			root = id
			break
		}
	}
	if root < 0 {
		return nil, fmt.Errorf("no page tree in the document")
	}
	var refs []int
	seen := map[int]bool{}
	var walk func(id int)
	walk = func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		obj, ok := objects[id]
		if !ok {
			return
		}
		switch {
		case isObjectType(obj.body, "Pages"):
			for _, kid := range kidsOf(obj.body) {
				walk(kid)
			}
		case isObjectType(obj.body, "Page"):
			refs = append(refs, id)
		}
	}
	walk(root)
	if len(refs) == 0 {
		return nil, fmt.Errorf("the page tree holds no page")
	}
	return refs, nil
}

// contentRef returns the object id of a page's /Contents stream, or -1 when the
// page declares none.
func contentRef(page []byte) int {
	idx := bytes.Index(page, []byte("/Contents"))
	if idx < 0 {
		return -1
	}
	return parseIntToken(page[idx+len("/Contents"):])
}

// isObjectType reports whether a dictionary names /Type /<name>. The byte after
// the name must not be a letter, so "Page" does not match "Pages".
func isObjectType(body []byte, name string) bool {
	marker := []byte("/Type /" + name)
	idx := bytes.Index(body, marker)
	if idx < 0 {
		return false
	}
	rest := body[idx+len(marker):]
	return len(rest) == 0 || !isASCIILetter(rest[0])
}

// isASCIILetter reports whether b is an ASCII letter, so a type name is not
// taken for a longer one that starts with it.
func isASCIILetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// kidsOf returns the page objects of a /Kids array, in written order.
func kidsOf(pagesNode []byte) []int {
	idx := bytes.Index(pagesNode, []byte("/Kids"))
	if idx < 0 {
		return nil
	}
	rest := pagesNode[idx:]
	open := bytes.IndexByte(rest, '[')
	shut := bytes.IndexByte(rest, ']')
	if open < 0 || shut < open {
		return nil
	}
	fields := bytes.Fields(rest[open+1 : shut])
	var kids []int
	for i := 0; i+2 < len(fields); {
		if string(fields[i+1]) != "0" || string(fields[i+2]) != "R" {
			i++
			continue
		}
		id, err := strconv.Atoi(string(fields[i]))
		if err != nil {
			i++
			continue
		}
		kids = append(kids, id)
		i += 3
	}
	return kids
}
