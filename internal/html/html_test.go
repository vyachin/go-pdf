package html

import (
	"strings"
	"testing"
)

func TestTokenizer(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []Token
	}{
		{
			name:  "start tag with attributes",
			input: `<p class="a" id='b'>`,
			want: []Token{
				{Kind: StartTagToken, Name: "p", Attrs: []Attr{{"class", "a"}, {"id", "b"}}},
			},
		},
		{
			name:  "text with entities",
			input: `x &amp; y &#1057;`,
			want: []Token{
				{Kind: TextToken, Data: "x & y С"},
			},
		},
		{
			name:  "self closing void element",
			input: `<br/>`,
			want: []Token{
				{Kind: StartTagToken, Name: "br", SelfClosing: true},
			},
		},
		{
			name:  "comment",
			input: `<!-- note -->`,
			want: []Token{
				{Kind: CommentToken, Data: " note "},
			},
		},
		{
			name:  "doctype",
			input: `<!DOCTYPE html>`,
			want: []Token{
				{Kind: DoctypeToken, Data: "html"},
			},
		},
		{
			name:  "raw text element",
			input: `<script>if (a<b) {}</script>`,
			want: []Token{
				{Kind: StartTagToken, Name: "script"},
				{Kind: TextToken, Data: "if (a<b) {}"},
				{Kind: EndTagToken, Name: "script"},
			},
		},
		{
			name:  "stray less than is text",
			input: `a < b`,
			want: []Token{
				{Kind: TextToken, Data: "a "},
				{Kind: TextToken, Data: "<"},
				{Kind: TextToken, Data: " b"},
			},
		},
		{
			name:  "unquoted attribute value",
			input: `<td colspan=2>`,
			want: []Token{
				{Kind: StartTagToken, Name: "td", Attrs: []Attr{{"colspan", "2"}}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := NewTokenizer([]byte(tc.input))
			var got []Token
			for {
				tok := z.Next()
				if tok.Kind == EOFToken {
					break
				}
				got = append(got, Token{Kind: tok.Kind, Name: tok.Name, Attrs: tok.Attrs, Data: tok.Data, SelfClosing: tok.SelfClosing})
				if tok.Kind == StartTagToken && (tok.Name == "script" || tok.Name == "style") {
					z.SetRawTag(tok.Name)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d tokens, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].Kind != tc.want[i].Kind || got[i].Name != tc.want[i].Name || got[i].Data != tc.want[i].Data || got[i].SelfClosing != tc.want[i].SelfClosing {
					t.Fatalf("token %d = %+v, want %+v", i, got[i], tc.want[i])
				}
				if len(got[i].Attrs) != len(tc.want[i].Attrs) {
					t.Fatalf("token %d attrs = %+v, want %+v", i, got[i].Attrs, tc.want[i].Attrs)
				}
				for j := range got[i].Attrs {
					if got[i].Attrs[j] != tc.want[i].Attrs[j] {
						t.Fatalf("token %d attr %d = %+v, want %+v", i, j, got[i].Attrs[j], tc.want[i].Attrs[j])
					}
				}
			}
		})
	}
}

func parseDump(t *testing.T, src string) string {
	t.Helper()
	doc, err := Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return Dump(doc.Root)
}

func TestParseTree(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "implicit html head body",
			input: `<p>hello</p>`,
			want: `#document
  <html>
    <head>
    <body>
      <p>
        "hello"
`,
		},
		{
			name:  "title goes to head",
			input: `<title>T</title><p>x</p>`,
			want: `#document
  <html>
    <head>
      <title>
        "T"
    <body>
      <p>
        "x"
`,
		},
		{
			name:  "implicit paragraph close",
			input: `<p>a<p>b`,
			want: `#document
  <html>
    <head>
    <body>
      <p>
        "a"
      <p>
        "b"
`,
		},
		{
			name:  "implicit list item close",
			input: `<ul><li>a<li>b</ul>`,
			want: `#document
  <html>
    <head>
    <body>
      <ul>
        <li>
          "a"
        <li>
          "b"
`,
		},
		{
			name:  "end tag closes intermediate elements",
			input: `<div><p>x</div>`,
			want: `#document
  <html>
    <head>
    <body>
      <div>
        <p>
          "x"
`,
		},
		{
			name:  "table text is foster parented",
			input: `<table>stray<tr><td>a</td></tr></table>`,
			want: `#document
  <html>
    <head>
    <body>
      "stray"
      <table>
        <tr>
          <td>
            "a"
`,
		},
		{
			name:  "explicit head and body",
			input: `<html><head><title>T</title></head><body class="b"><h1>H</h1></body></html>`,
			want: `#document
  <html>
    <head>
      <title>
        "T"
    <body class="b">
      <h1>
        "H"
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDump(t, tc.input)
			if got != tc.want {
				t.Fatalf("tree mismatch\ngot:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	src := "<p>\nhello</p>"
	doc, err := Parse([]byte(src), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	body := doc.Body()
	if body == nil {
		t.Fatal("no body")
	}
	p := body.FirstElementChild()
	if p == nil || p.Tag != "p" {
		t.Fatalf("expected p, got %+v", p)
	}
	pos := doc.Pos(p)
	if pos.File != "doc.html" || pos.Line != 1 || pos.Col != 1 {
		t.Fatalf("p position = %+v, want doc.html:1:1", pos)
	}
	textPos := doc.Position(strings.Index(src, "hello"))
	if textPos.Line != 2 || textPos.Col != 1 {
		t.Fatalf("text position = %+v, want line 2 col 1", textPos)
	}
}

func TestElementPath(t *testing.T) {
	doc, err := Parse([]byte(`<body><div class="invoice"><p id="total">x</p></div></body>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.FindByID("total")
	if p == nil {
		t.Fatal("no element with id total")
	}
	if got, want := p.Path(), "body > div.invoice > p#total"; got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestTextContent(t *testing.T) {
	doc, err := Parse([]byte(`<p>a <strong>b</strong> c</p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Body().FirstElementChild()
	if got, want := p.TextContent(), "a b c"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("<p>hi</p>"))
	f.Add([]byte("<table><tr><td>x</table>"))
	f.Add([]byte("<!-- unterminated"))
	f.Add([]byte("<p class=\"a\" id='b'>x &amp; y</p>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if _, err := Parse(b, "fuzz.html"); err != nil {
			t.Fatalf("Parse returned an error: %v", err)
		}
	})
}
