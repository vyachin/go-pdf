package html

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestImpliedEndTagsDoNotHideDepth guards the exact depth walk: the token
// stream never nests, but the standard tree does, so the limit must be decided
// on the tree.
func TestImpliedEndTagsDoNotHideDepth(t *testing.T) {
	src := []byte(strings.Repeat("<p><div></p>", 20))
	if _, err := ParseWithOptions(src, ParseOptions{MaxDepth: 8}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the implied-end-tag document returned %v, want a budget limit error", err)
	}
}

// TestImpliedEndTagsDoNotInventDepth guards the other direction: 300 sibling
// paragraphs with omitted end tags are a flat tree under a limit of 256.
func TestImpliedEndTagsDoNotInventDepth(t *testing.T) {
	src := []byte(strings.Repeat("<p>text", 300))
	if _, err := ParseWithOptions(src, ParseOptions{MaxDepth: 256}); err != nil {
		t.Fatalf("300 sibling paragraphs were rejected: %v", err)
	}
}

// TestDepthProbeStopsAtTheBudget guards that the exact walk charges the shared
// guard and observes cancellation.
func TestDepthProbeStopsAtTheBudget(t *testing.T) {
	src := []byte(strings.Repeat("<p>text</p>", 200))
	guard := budget.New(context.Background(), 10)
	if _, err := ParseWithOptions(src, ParseOptions{MaxDepth: 256, Budget: guard}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the depth walk returned %v, want a budget limit error", err)
	}
}

// TestRepeatedStructuralElements guards that merged body/head/main and implied
// tbody do not enlarge the depth past the real tree.
func TestRepeatedStructuralElements(t *testing.T) {
	cases := []string{
		`<body><body><main><main>text`,
		`<head><head><title>t</title></head><body><main>text`,
		`<table><tr><td>a<tr><td>b`,
		`<table><tbody><tr><td>a<td>b`,
	}
	for _, src := range cases {
		if _, err := ParseWithOptions([]byte(src), ParseOptions{MaxDepth: 8}); err != nil {
			t.Fatalf("%q was rejected: %v", src, err)
		}
	}
}

// TestManyDistinctTagNames guards the intern table: a flat document with more
// names than the old 65,536-entry table must not report a nesting error.
func TestManyDistinctTagNames(t *testing.T) {
	const names = 70_000
	var b strings.Builder
	for i := 0; i < names; i++ {
		fmt.Fprintf(&b, "<x%d></x%d>", i, i)
	}
	b.WriteString("<target>text</target>")
	src := b.String()
	doc, err := Parse([]byte(src), "names.html")
	if err != nil {
		t.Fatalf("the flat document with %d names failed: %v", names, err)
	}
	if doc.Root == nil || len(doc.Root.Children) == 0 {
		t.Fatal("the document has no tree")
	}
	// A silent shared index made the last element inherit the first start-tag
	// span; the source position proves the name got its own entry.
	var target *Node
	var find func(*Node)
	find = func(n *Node) {
		if n.Type == ElementNode && n.Tag == "target" {
			target = n
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(doc.Root)
	if target == nil {
		t.Fatal("the target element is missing")
	}
	if want := strings.Index(src, "<target>"); target.Start != want {
		t.Fatalf("target Start = %d, want %d", target.Start, want)
	}
}

// TestTreeConstructionDepthLimitIsCoded guards the third-party open-stack
// limit: it must become a resource limit instead of a raw error or a panic.
func TestTreeConstructionDepthLimitIsCoded(t *testing.T) {
	src := []byte(strings.Repeat("<b><div></b>", 600))
	_, err := ParseWithOptions(src, ParseOptions{MaxDepth: 10_000})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the open-stack document returned %v, want a budget limit error", err)
	}
	if _, err := Parse(src, "open-stack.html"); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("Parse returned %v, want a budget limit error", err)
	}
}

// TestExplicitCharsetStripsBOM guards the explicit-encoding path: a UTF-8 mark
// must not decode as visible "п»ї" text when the caller names windows-1251.
func TestExplicitCharsetStripsBOM(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte("<p>x</p>")...)
	doc, err := ParseWithOptions(src, ParseOptions{Charset: "windows-1251"})
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Body().TextContent(); got != "x" {
		t.Fatalf("body text = %q, want x", got)
	}
}

// TestDepthMatchesTheStandardTree is the differential oracle: for every shape
// whose tree differs from the token stream, the enforced depth must equal the
// depth of the tree the standard construction algorithm produces.
func TestDepthMatchesTheStandardTree(t *testing.T) {
	cases := []string{
		`<p>a<p>b`,
		`<p><div></p><p><div></p>`,
		`<ul><li>a<li>b</ul>`,
		`<table><tr><td>a<td>b`,
		`<html><html><body><body><main><main>x`,
		`</p></div></span>text`,
		`<script>if (a<b) {}</script><style>p{}</style>text`,
		`<table><b>x</b><tr><td>y`,
		`<b><i></b></i>text`,
		`<select><option>a<option>b</select>`,
	}
	for _, src := range cases {
		tree, err := xhtml.Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		depth := standardDepth(tree)
		if depth < 1 {
			t.Fatalf("%q: computed depth %d", src, depth)
		}
		if _, err := ParseWithOptions([]byte(src), ParseOptions{MaxDepth: depth}); err != nil {
			t.Fatalf("%q at the real depth %d failed: %v", src, depth, err)
		}
		if _, err := ParseWithOptions([]byte(src), ParseOptions{MaxDepth: depth - 1}); !errors.Is(err, budget.ErrExceeded) {
			t.Fatalf("%q at depth %d returned %v, want a limit error", src, depth-1, err)
		}
	}
}

// standardDepth measures the deepest element of a standard tree.
func standardDepth(root *xhtml.Node) int {
	type item struct {
		node  *xhtml.Node
		depth int
	}
	deepest := 0
	stack := []item{{root, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		depth := it.depth
		if it.node.Type == xhtml.ElementNode {
			depth++
			if depth > deepest {
				deepest = depth
			}
		}
		for c := it.node.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, item{c, depth})
		}
	}
	return deepest
}
