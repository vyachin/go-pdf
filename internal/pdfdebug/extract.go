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
	"strconv"
	"strings"
)

// Pages returns the extracted text of every page, in page order.
func Pages(data []byte) ([]string, error) {
	objects := splitObjects(data)
	toUni := buildToUni(objects)
	type entry struct {
		id   int
		text string
	}
	var pages []entry
	for id, obj := range objects {
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
		if !bytes.Contains(stream, []byte("Tj")) && !bytes.Contains(stream, []byte("TJ")) {
			continue
		}
		pages = append(pages, entry{id: id, text: textFromStream(stream, toUni)})
	}
	// The writer allocates content streams in page order.
	sortEntries(pages, func(a, b entry) bool { return a.id < b.id })
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.text
	}
	return out, nil
}

// ContentStreams returns the decoded content streams of the document. It is
// used by tests that assert on emitted operators rather than on text.
func ContentStreams(data []byte) [][]byte {
	var out [][]byte
	for _, obj := range splitObjects(data) {
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
		if bytes.Contains(stream, []byte("Tj")) || bytes.Contains(stream, []byte("TJ")) || bytes.Contains(stream, []byte(" re ")) {
			out = append(out, stream)
		}
	}
	return out
}

// Text returns the text of the whole document with form feed separators
// between pages, similar to pdftotext.
func Text(data []byte) (string, error) {
	pages, err := Pages(data)
	if err != nil {
		return "", err
	}
	return strings.Join(pages, "\f\n"), nil
}

func sortEntries[T any](items []T, less func(a, b T) bool) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && less(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
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
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildToUni(objects map[int]*pdfObject) map[uint16]rune {
	toUni := map[uint16]rune{}
	for _, obj := range objects {
		if !bytes.Contains(obj.body, []byte("/ToUnicode")) {
			continue
		}
		idx := bytes.Index(obj.body, []byte("/ToUnicode"))
		ref := parseIntToken(obj.body[idx+len("/ToUnicode"):])
		target, ok := objects[ref]
		if !ok {
			continue
		}
		stream, err := decodedStream(target.body)
		if err != nil {
			continue
		}
		parseBFChar(stream, toUni)
	}
	return toUni
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

func decodeUTF16Hex(s string) []rune {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw)%2 != 0 {
		return nil
	}
	var runes []rune
	for i := 0; i+1 < len(raw); i += 2 {
		runes = append(runes, rune(uint16(raw[i])<<8|uint16(raw[i+1])))
	}
	return runes
}

// textFromStream reads both plain Tj strings and TJ arrays. Adjustments in a
// TJ array only move glyphs; extraction ignores them.
func textFromStream(stream []byte, toUni map[uint16]rune) string {
	var out strings.Builder
	s := string(stream)
	inArray := false
	for i := 0; i < len(s); {
		switch {
		case s[i] == '[':
			inArray = true
			i++
		case s[i] == ']':
			inArray = false
			i++
		case s[i] == '<':
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				return out.String()
			}
			hexStr := s[i+1 : i+end]
			i += end + 1
			if inArray {
				out.WriteString(decodeHexText(hexStr, toUni))
				continue
			}
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\r' || s[j] == '\t') {
				j++
			}
			if strings.HasPrefix(s[j:], "Tj") {
				out.WriteString(decodeHexText(hexStr, toUni))
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
