package charset

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// encode returns s in the given encoding; the tests build their sources with
// the same tables the package decodes with.
func encode(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encoding %q: %v", s, err)
	}
	return out
}

func cp1251(t *testing.T, s string) []byte { t.Helper(); return encode(t, charmap.Windows1251, s) }
func koi8r(t *testing.T, s string) []byte  { t.Helper(); return encode(t, charmap.KOI8R, s) }
func ibm866(t *testing.T, s string) []byte { t.Helper(); return encode(t, charmap.CodePage866, s) }

func TestResolve(t *testing.T) {
	utf8BOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte("<p>Привет</p>")...)
	utf16BOM := []byte{0xFF, 0xFE, '<', 0, 'p', 0, '>', 0}
	cases := []struct {
		name     string
		src      []byte
		explicit string
		want     string // canonical name, when no error is expected
		errHas   []string
		errAt    int // expected Offset, when >= -1; -2 means do not check
	}{
		{name: "pure ascii", src: []byte("<p>hi</p>"), want: UTF8, errAt: -2},
		{name: "empty document", src: nil, want: UTF8, errAt: -2},
		{name: "utf-8 text without declaration", src: []byte("<p>Привет</p>"), want: UTF8, errAt: -2},
		{name: "utf-8 byte order mark", src: utf8BOM, want: UTF8, errAt: -2},
		{name: "declared windows-1251", src: cp1251(t, `<meta charset="windows-1251"><p>Счёт</p>`), want: Windows1251, errAt: -2},
		{name: "http-equiv form", src: cp1251(t, `<meta http-equiv="Content-Type" content="text/html; charset=windows-1251"><p>Счёт</p>`), want: Windows1251, errAt: -2},
		{name: "declared koi8-r", src: koi8r(t, `<meta charset="koi8-r"><p>Счёт</p>`), want: KOI8R, errAt: -2},
		{name: "declared ibm866", src: ibm866(t, `<meta charset="ibm866"><p>Счёт</p>`), want: IBM866, errAt: -2},
		{name: "explicit alias", src: cp1251(t, `<p>Счёт</p>`), explicit: "cp1251", want: Windows1251, errAt: -2},
		{name: "explicit beats declaration", src: cp1251(t, `<meta charset="koi8-r">Счёт`), explicit: "windows-1251", want: Windows1251, errAt: -2},
		{name: "explicit utf-8", src: []byte("<p>Привет</p>"), explicit: "UTF-8", want: UTF8, errAt: -2},
		{
			name:   "undeclared legacy document",
			src:    []byte("<p>" + string(cp1251(t, "Счёт")) + "</p>"),
			errHas: []string{"windows-1252 is not supported", Windows1251, KOI8R, IBM866, "Options.Charset"},
			errAt:  3,
		},
		{
			name:   "unsupported declaration",
			src:    []byte(`<meta charset="shift_jis"><p>x</p>`),
			errHas: []string{"shift_jis", "unsupported document encoding"},
			errAt:  0,
		},
		{
			name:   "unsupported declaration names the meta",
			src:    []byte("<html>\n<head>\n<meta charset=\"gbk\">\n</head>"),
			errHas: []string{"gbk"},
			errAt:  14,
		},
		{
			name:   "utf-16 byte order mark",
			src:    utf16BOM,
			errHas: []string{"UTF-16"},
			errAt:  0,
		},
		{
			name:   "declared windows-1252",
			src:    append([]byte(`<meta charset="windows-1252">`), cp1251(t, "Счёт")...),
			errHas: []string{"windows-1252", Windows1251, "Options.Charset"},
			errAt:  0,
		},
		{
			name:   "ascii document declaring windows-1252",
			src:    []byte(`<meta charset="windows-1252"><p>x</p>`),
			errHas: []string{"unsupported document encoding", "windows-1252"},
			errAt:  0,
		},
		{
			name:     "explicit unsupported label",
			src:      []byte("<p>x</p>"),
			explicit: "shift_jis",
			errHas:   []string{"unsupported charset", "shift_jis"},
			errAt:    -1,
		},
		{
			name:     "explicit unknown label",
			src:      []byte("<p>x</p>"),
			explicit: "nonsense",
			errHas:   []string{"unknown charset", "nonsense"},
			errAt:    -1,
		},
		{
			name:     "explicit windows-1252 is refused",
			src:      []byte("<p>x</p>"),
			explicit: "windows-1252",
			errHas:   []string{"unsupported charset", "windows-1252"},
			errAt:    -1,
		},
		{
			name:     "explicit utf-16 is refused",
			src:      []byte("<p>x</p>"),
			explicit: "utf-16",
			errHas:   []string{"unsupported charset"},
			errAt:    -1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := Resolve(tc.src, tc.explicit)
			if tc.want != "" {
				if err != nil {
					t.Fatalf("Resolve: %v", err)
				}
				if enc.Name() != tc.want {
					t.Fatalf("encoding = %q, want %q", enc.Name(), tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Resolve returned %q, want an error", enc.Name())
			}
			for _, want := range tc.errHas {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error %T is not a *charset.Error", err)
			}
			if tc.errAt != -2 && e.Offset != tc.errAt {
				t.Fatalf("Offset = %d, want %d (message: %s)", e.Offset, tc.errAt, e)
			}
			if tc.errAt >= 0 && (e.Line == 0 || e.Col == 0) {
				t.Fatalf("error %q has no position", e)
			}
		})
	}
}

// TestDetectIgnoresCommentedAndLateDeclarations covers the corners of the
// standard library's prescan that the package relies on.
func TestDetectIgnoresCommentedAndLateDeclarations(t *testing.T) {
	body := cp1251(t, "Счёт")
	commented := append([]byte(`<!-- <meta charset="windows-1251"> --><p>`), body...)
	if enc, err := Resolve(commented, ""); err == nil {
		t.Fatalf("a declaration inside a comment must be ignored, resolved to %q", enc.Name())
	}

	// The declaration must fit inside the first 1024 bytes.
	inside := append(bytes.Repeat([]byte(" "), 990), []byte(`<meta charset="windows-1251"><p>`)...)
	inside = append(inside, body...)
	if enc, err := Resolve(inside, ""); err != nil || enc.Name() != Windows1251 {
		t.Fatalf("a declaration ending inside 1024 bytes must be found: %v, %v", enc, err)
	}
	late := append(bytes.Repeat([]byte(" "), 1100), []byte(`<meta charset="windows-1251"><p>`)...)
	late = append(late, body...)
	if _, err := Resolve(late, ""); err == nil {
		t.Fatal("a declaration after the first 1024 bytes must be ignored")
	}
}

// TestDeclaredCharset covers the prescan's shape rules: only a real <meta>
// declaration counts, and the offset is the tag's '<' in the original bytes.
func TestDeclaredCharset(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	const flag = "windows-1251"
	cases := []struct {
		name   string
		src    []byte
		label  string
		offset int
	}{
		{name: "no declaration", src: []byte("<p>hi</p>")},
		{name: "genuine charset", src: []byte(`<meta charset="windows-1251">`), label: flag},
		{name: "uppercase tag and name", src: []byte(`<META CHARSET="Windows-1251">`), label: "Windows-1251"},
		{name: "single-quoted value", src: []byte(`<meta charset='windows-1251'>`), label: flag},
		{name: "unquoted value", src: []byte(`<meta charset=windows-1251>`), label: flag},
		{name: "space around equals", src: []byte(`<meta charset = windows-1251>`), label: flag},
		{name: "greater-than in a quoted value", src: []byte(`<meta name="description" content="a > b" charset="windows-1251">`), label: flag},
		{name: "empty charset", src: []byte(`<meta charset="">`)},
		{name: "charset in another attribute", src: []byte(`<meta name="description" content="charset is a word">`)},
		{name: "markup in another attribute", src: []byte(`<meta name="x" content='<meta charset="utf-8">'>`)},
		{name: "charset attribute on another element", src: []byte(`<div charset="windows-1251"></div>`)},
		{name: "meta prefix of another element", src: []byte(`<metadata charset="windows-1251">`)},
		{name: "comment", src: []byte(`<!-- <meta charset="utf-8"> -->`)},
		{
			name:   "comment then real declaration",
			src:    []byte(`<!-- <meta charset="utf-8"> --><meta charset="windows-1251">`),
			label:  flag,
			offset: len(`<!-- <meta charset="utf-8"> -->`),
		},
		{
			name:   "doctype",
			src:    []byte(`<!DOCTYPE html><meta charset="windows-1251">`),
			label:  flag,
			offset: len(`<!DOCTYPE html>`),
		},
		{
			name:   "processing instruction",
			src:    []byte(`<?xml version="1.0"?><meta charset="windows-1251">`),
			label:  flag,
			offset: len(`<?xml version="1.0"?>`),
		},
		{name: "unterminated comment", src: []byte(`<!-- <meta charset="windows-1251">`)},
		{name: "unterminated tag", src: []byte(`<meta charset="windows-1251"`)},
		{name: "non-meta element before the declaration", src: []byte(`<p title="a > b">x</p><meta charset="windows-1251">`), label: flag, offset: len(`<p title="a > b">x</p>`)},
		{
			name:  "http-equiv",
			src:   []byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`),
			label: flag,
		},
		{
			name:  "http-equiv reversed attributes",
			src:   []byte(`<meta content="text/html; charset=windows-1251" http-equiv=content-type>`),
			label: flag,
		},
		{
			name:  "http-equiv quoted label",
			src:   []byte(`<meta http-equiv="Content-Type" content='text/html; charset="windows-1251"'>`),
			label: flag,
		},
		{name: "http-equiv other type", src: []byte(`<meta http-equiv="refresh" content="0; charset=windows-1251">`)},
		{name: "http-equiv without content", src: []byte(`<meta http-equiv="Content-Type">`)},
		{name: "byte order mark", src: append(append([]byte{}, bom...), []byte(`<meta charset="windows-1251">`)...), label: flag, offset: len(bom)},
		{name: "declaration after the window", src: append(bytes.Repeat([]byte(" "), prescanWindow), []byte(`<meta charset="windows-1251">`)...)},
		{name: "tag cut by the window", src: append(bytes.Repeat([]byte(" "), prescanWindow-24), []byte(`<meta charset="windows-1251">`)...)},
		{
			name:   "tag ends at the window",
			src:    append(bytes.Repeat([]byte(" "), prescanWindow-len(`<meta charset="windows-1251">`)), []byte(`<meta charset="windows-1251">`)...),
			label:  flag,
			offset: prescanWindow - len(`<meta charset="windows-1251">`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, offset := declaredCharset(tc.src)
			wantOffset := tc.offset
			if tc.label == "" {
				wantOffset = -1
			}
			if label != tc.label || offset != wantOffset {
				t.Fatalf("declaredCharset = %q at %d, want %q at %d", label, offset, tc.label, wantOffset)
			}
		})
	}
}

// TestResolveIgnoresNonDeclarations proves the prescan does not let a comment
// or another attribute value decide the encoding: such a document is
// undecodable at its first invalid byte, and no supported encoding is named as
// unsupported.
func TestResolveIgnoresNonDeclarations(t *testing.T) {
	body := cp1251(t, "Счёт")
	cases := []struct {
		name   string
		prefix string
	}{
		{name: "comment", prefix: `<!-- <meta charset="utf-8"> --><p>`},
		{name: "description attribute", prefix: `<meta name="description" content="charset is a word"><p>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := append([]byte(tc.prefix), body...)
			_, err := Resolve(src, "")
			if err == nil {
				t.Fatal("Resolve accepted a document with no real declaration")
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error %T is not a *charset.Error", err)
			}
			if strings.Contains(e.Msg, "unsupported document encoding") {
				t.Fatalf("the diagnostic names an encoding the document does not declare: %q", e.Msg)
			}
			if strings.Contains(e.Msg, `"utf-8"`) || strings.Contains(e.Msg, `"is"`) {
				t.Fatalf("the diagnostic quotes ignored markup as an encoding: %q", e.Msg)
			}
			if e.Offset != len(tc.prefix) || e.Line != 1 || e.Col != len(tc.prefix)+1 {
				t.Fatalf("position = offset %d, %d:%d; want %d at 1:%d", e.Offset, e.Line, e.Col, len(tc.prefix), len(tc.prefix)+1)
			}
		})
	}
}

// TestDeclarationPositionInOriginalBytes proves the offset and column named
// for a declaration are the author's, even after a multi-byte word and a
// newline.
func TestDeclarationPositionInOriginalBytes(t *testing.T) {
	src := []byte("<p>Привет</p>\n  <meta charset=\"big5\">")
	wantOffset := bytes.Index(src, []byte("<meta"))
	_, err := Resolve(src, "")
	if err == nil {
		t.Fatal("an unsupported declaration was accepted")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not a *charset.Error", err)
	}
	if e.Offset != wantOffset || e.Line != 2 || e.Col != 3 {
		t.Fatalf("position = offset %d, %d:%d; want offset %d at 2:3", e.Offset, e.Line, e.Col, wantOffset)
	}
}

// TestDeclaredCharsetBoundsMemory guards the prescan window: a megabyte of
// markup-free bytes is scanned without allocating per byte.
func TestDeclaredCharsetBoundsMemory(t *testing.T) {
	src := bytes.Repeat([]byte("<"), 1<<20)
	allocs := testing.AllocsPerRun(1, func() { declaredCharset(src) })
	if allocs > 8 {
		t.Fatalf("a megabyte of '<' allocated %v objects", allocs)
	}
}

// FuzzDeclaredCharset seeds the scanner with the comment and attribute shapes
// the prescan must tolerate, and checks that a reported offset always lies in
// the bytes it was given.
func FuzzDeclaredCharset(f *testing.F) {
	for _, seed := range []string{
		`<!-- <meta charset="utf-8"> -->`,
		`<meta name="description" content="charset is a word">`,
		`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`,
		`<meta charset=windows-1251>`,
		`<meta content="a > b" charset="windows-1251">`,
		`<meta name="x" content='<meta charset="utf-8">'>`,
		`<!DOCTYPE html><meta charset="windows-1251">`,
		`<?xml version="1.0"?><meta charset="windows-1251">`,
		`<!-- <meta charset="windows-1251">`,
		`<meta charset="windows-1251"`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		label, offset := declaredCharset(src)
		if offset < -1 || offset > len(src) {
			t.Fatalf("offset %d outside %d bytes", offset, len(src))
		}
		if label == "" && offset != -1 {
			t.Fatalf("empty label with offset %d", offset)
		}
		if label != "" && (offset < 0 || offset >= len(src)) {
			t.Fatalf("label %q at offset %d in %d bytes", label, offset, len(src))
		}
	})
}

func TestTables(t *testing.T) {
	wantUndefined := map[string][]int{
		Windows1251: {0x98},
		KOI8R:       nil,
		IBM866:      nil,
	}
	for _, name := range []string{Windows1251, KOI8R, IBM866} {
		enc, err := Validate(name)
		if err != nil {
			t.Fatal(err)
		}
		if enc.table == nil {
			t.Fatalf("%s has no table", name)
		}
		var undefined []int
		for b := 0; b < 256; b++ {
			entry := enc.table[b]
			if entry == nil {
				undefined = append(undefined, b)
				continue
			}
			if len(entry) < 1 || len(entry) > 3 {
				t.Fatalf("%s byte %#02x decoded to %d bytes", name, b, len(entry))
			}
			if !utf8.Valid(entry) || bytes.Equal(entry, replacement) {
				t.Fatalf("%s byte %#02x decoded to %q", name, b, entry)
			}
		}
		if len(undefined) != len(wantUndefined[name]) {
			t.Fatalf("%s undefined bytes = %v, want %v", name, undefined, wantUndefined[name])
		}
		for i := range undefined {
			if undefined[i] != wantUndefined[name][i] {
				t.Fatalf("%s undefined bytes = %v, want %v", name, undefined, wantUndefined[name])
			}
		}
	}
	// Every ASCII byte is itself in every supported encoding.
	for _, name := range []string{Windows1251, KOI8R, IBM866} {
		enc, err := Validate(name)
		if err != nil {
			t.Fatal(err)
		}
		for b := 0; b < 0x80; b++ {
			if len(enc.table[b]) != 1 || enc.table[b][0] != byte(b) {
				t.Fatalf("%s byte %#02x decoded to %q", name, b, enc.table[b])
			}
		}
	}
}

// TestTablesAreShared proves the table is built once and the same immutable
// value is handed to every conversion.
func TestTablesAreShared(t *testing.T) {
	first, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve([]byte(`<meta charset="windows-1251">`), "")
	if err != nil {
		t.Fatal(err)
	}
	if first.table != second.table {
		t.Fatal("two resolutions of one encoding built two tables")
	}
}

func TestDecode(t *testing.T) {
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	src := cp1251(t, "Счёт")
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := string(dec.Text), "Счёт"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	wantWidths := []uint8{2, 2, 2, 2}
	if len(dec.Widths) != len(wantWidths) {
		t.Fatalf("widths = %v, want %v", dec.Widths, wantWidths)
	}
	for i := range wantWidths {
		if dec.Widths[i] != wantWidths[i] {
			t.Fatalf("widths = %v, want %v", dec.Widths, wantWidths)
		}
	}
	if dec.Skip != 0 {
		t.Fatalf("skip = %d, want 0", dec.Skip)
	}

	// ASCII is one byte in and one byte out under a legacy encoding.
	dec, err = Decode(enc, []byte("<p>hi</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if string(dec.Text) != "<p>hi</p>" {
		t.Fatalf("text = %q", dec.Text)
	}
	for i, w := range dec.Widths {
		if w != 1 {
			t.Fatalf("width %d = %d, want 1", i, w)
		}
	}
}

func TestDecodeUndefinedByte(t *testing.T) {
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	src := append(cp1251(t, "Счёт\n"), 0x98)
	_, err = Decode(enc, src)
	if err == nil {
		t.Fatal("byte 0x98 must be rejected")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not a *charset.Error", err)
	}
	if e.Offset != 5 || e.Line != 2 || e.Col != 1 {
		t.Fatalf("position = offset %d, line %d, col %d; want 5, 2, 1", e.Offset, e.Line, e.Col)
	}
	if !strings.Contains(err.Error(), "0x98") || !strings.Contains(err.Error(), Windows1251) {
		t.Fatalf("error %q does not name the byte and the encoding", err)
	}
}

func TestDecodeUTF8(t *testing.T) {
	enc, err := Validate(UTF8)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("<p>Привет</p>")
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	if &dec.Text[0] != &src[0] {
		t.Fatal("a UTF-8 document without a mark must not be copied")
	}
	if dec.Widths != nil || dec.Skip != 0 {
		t.Fatalf("widths = %v, skip = %d", dec.Widths, dec.Skip)
	}

	// A byte order mark is removed and does not shift a position.
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, src...)
	dec, err = Decode(enc, withBOM)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec.Text) != string(src) || dec.Skip != 3 {
		t.Fatalf("text = %q, skip = %d", dec.Text, dec.Skip)
	}

	// Invalid bytes are not replaced with U+FFFD.
	bad := []byte("<p>\xC3</p>")
	if _, err := Decode(enc, bad); err == nil {
		t.Fatal("an invalid UTF-8 byte must be rejected")
	} else {
		var e *Error
		if !errors.As(err, &e) || e.Offset != 3 || e.Line != 1 || e.Col != 4 {
			t.Fatalf("error = %v, want offset 3 at 1:4", err)
		}
	}
}

func TestByteOrderMark(t *testing.T) {
	for _, tc := range []struct {
		src  []byte
		skip int
	}{
		{[]byte("<p>"), 0},
		{[]byte{0xEF, 0xBB, 0xBF, '<'}, 3},
		{[]byte{0xEF, 0xBB}, 0},
	} {
		if skip, _ := byteOrderMark(tc.src); skip != tc.skip {
			t.Fatalf("byteOrderMark(%q) = %d, want %d", tc.src, skip, tc.skip)
		}
	}
}

// TestDecodeExplicitEncodingStripsUTF8BOM guards that a caller-named legacy
// encoding does not render a UTF-8 byte order mark as "п»ї".
func TestDecodeExplicitEncodingStripsUTF8BOM(t *testing.T) {
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("<p>text</p>")
	src := append([]byte{0xEF, 0xBB, 0xBF}, body...)
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec.Text) != string(body) || dec.Skip != 3 {
		t.Fatalf("text = %q, skip = %d", dec.Text, dec.Skip)
	}
	if len(dec.Widths) != len(src) {
		t.Fatalf("widths = %d bytes, want %d", len(dec.Widths), len(src))
	}
	m := NewMapper(src, dec.Text, dec.Widths, dec.Skip)
	if line, col := m.Position(0); line != 1 || col != 1 {
		t.Fatalf("position 0 = %d:%d, want 1:1", line, col)
	}
}

// TestMetaOffsetSurvivesCaseFolding guards RV4-38: firstMetaOffset used
// bytes.ToLower, which changes the byte length of a rune whose lowercase form
// is longer, so the reported column shifted. Folding ASCII only keeps the byte
// offset of the declaration exact.
func TestMetaOffsetSurvivesCaseFolding(t *testing.T) {
	cases := map[string][]byte{
		"after Cyrillic": []byte("<p>Привет</p><meta charset=\"big5\">"),
		"after dotted I": []byte("<p>İstanbul</p><meta charset=\"big5\">"),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			want := bytes.Index(src, []byte("<meta"))
			if want < 0 {
				t.Fatal("the fixture has no meta tag")
			}
			_, err := Resolve(src, "")
			if err == nil {
				t.Fatal("an unsupported declaration was accepted")
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error %T is not a *charset.Error", err)
			}
			if e.Offset != want {
				t.Fatalf("the diagnostic points at byte %d, want %d", e.Offset, want)
			}
		})
	}
}
