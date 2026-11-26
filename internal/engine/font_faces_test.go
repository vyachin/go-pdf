package engine

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/layout"
)

// fontFacesCSS declares two families whose layout caches collided before
// RV4-43: the single name "a b" and the list "a", "b" both format as "a b"
// through %v. Each family has its own bundled face, so a collision is visible
// as the wrong subfamily rather than merely a wrong number of fonts.
const fontFacesCSS = `
	@font-face { font-family: "a b"; src: url(DejaVuSans.ttf) }
	@font-face { font-family: "a"; src: url(DejaVuSans-Bold.ttf) }
`

// fontRequest is one font request of the style matrix: a family list plus the
// weight, slant and size that together form the registry and layout cache keys.
type fontRequest struct {
	families  []string
	weight    int
	italic    bool
	size      int32
	subfamily string // the bundled face the request must resolve to
}

// newTestFontRegistry builds a registry over fontFacesCSS and the bundled
// fonts. The registry owns a fresh asset registry, closed when the test ends.
func newTestFontRegistry(t *testing.T, sheetCSS string) *fontRegistry {
	t.Helper()
	ctx := context.Background()
	guard := budget.New(ctx, 0)
	reg := assets.New(guard)
	t.Cleanup(func() { _ = reg.Close() })
	sheet := css.ParseSheet(css.SheetOptions{File: "faces.css"}, []byte(sheetCSS))
	r, err := newFontRegistry(ctx, guard, []*css.Stylesheet{sheet}, Options{
		FontDir: filepath.Join(repoRoot(), "testdata", "fonts"),
	}, reg)
	if err != nil {
		t.Fatalf("building the font registry: %v", err)
	}
	return r
}

// layoutFaces prepares and lays out a document and returns the face layout
// assigned to each painted text run, in painting order. The documents below
// put one single-word paragraph per request, so the order of the returned
// faces is the order of the paragraphs.
func layoutFaces(t *testing.T, src string, opts Options) []*layout.FontFace {
	t.Helper()
	p, diags, err := prepare(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if p == nil {
		t.Fatalf("the document is not valid:\n%s", strings.Join(diags.Lines(), "\n"))
	}
	defer func() { _ = p.assets.Close() }()
	return layoutPrepared(t, p, opts, &diags)
}

// layoutPrepared lays out an already prepared document and returns the text
// faces in painting order. Running it twice over one prepared document warms
// the registry caches without changing the document.
func layoutPrepared(t *testing.T, p *prepared, opts Options, diags *diag.List) []*layout.FontFace {
	t.Helper()
	pages, err := layoutDocument(context.Background(), p, opts, diags)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("layout failed:\n%s", strings.Join(diags.Lines(), "\n"))
	}
	var faces []*layout.FontFace
	for _, page := range pages.Pages {
		for _, op := range page.Ops {
			if op.Kind == layout.TextOpKind && op.Text != nil {
				faces = append(faces, op.Text.Face)
			}
		}
	}
	return faces
}

// faceSnapshot is the observable result of one resolution: the resolved face,
// its metrics at the requested size and the cache key it was stored under.
type faceSnapshot struct {
	family    string
	subfamily string
	upem      int32
	glyphs    uint16
	size      int32
	ascent    int32
	descent   int32
}

func snapshotFace(face *layout.FontFace) faceSnapshot {
	return faceSnapshot{
		family:    face.Font.Family(),
		subfamily: face.Font.Subfamily(),
		upem:      face.Font.UnitsPerEm(),
		glyphs:    face.Font.NumGlyphs(),
		size:      face.Size,
		ascent:    face.Ascent,
		descent:   face.Descent,
	}
}

// familyCSS renders a family list as a declaration value: every name is
// quoted, so "a b" stays one family and "a", "b" stays a list.
func familyCSS(families []string) string {
	quoted := make([]string, len(families))
	for i, family := range families {
		quoted[i] = strconv.Quote(family)
	}
	return strings.Join(quoted, ", ")
}

// fontRequestDocument renders one single-word paragraph per request, in the
// given order, so layout paints them in that order.
func fontRequestDocument(requests []fontRequest, order []int) string {
	var b strings.Builder
	b.WriteString("<style>")
	b.WriteString(fontFacesCSS)
	b.WriteString("</style>")
	for _, i := range order {
		req := requests[i]
		style := "normal"
		if req.italic {
			style = "italic"
		}
		fmt.Fprintf(&b, `<p style='font-family: %s; font-weight: %d; font-style: %s; font-size: %gpt'>r%d</p>`,
			familyCSS(req.families), req.weight, style, float64(req.size)/1000, i)
	}
	return b.String()
}

// TestFaceCacheDistinguishesFamilyLists guards RV4-43: ["a b"] and ["a", "b"]
// printed the same through %v, so the second request received the first
// family's face. Both lists must resolve to their own bundled face through the
// registry cache and through the layout cache, in both request orders, with
// cold and warm caches. Unlike the previous fixture there is no extra fallback
// family: the two lists are exactly the colliding spellings.
func TestFaceCacheDistinguishesFamilyLists(t *testing.T) {
	fontsDir := filepath.Join(repoRoot(), "testdata", "fonts")
	requests := []fontRequest{
		{families: []string{"a b"}, weight: 400, size: 12000, subfamily: "Book"},
		{families: []string{"a", "b"}, weight: 400, size: 12000, subfamily: "Bold"},
	}
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		order := order
		t.Run(fmt.Sprintf("order %v", order), func(t *testing.T) {
			// Registry cache: the first pass fills it and the second is warm.
			r := newTestFontRegistry(t, fontFacesCSS)
			cold := map[int]*layout.FontFace{}
			for pass := 0; pass < 2; pass++ {
				for _, i := range order {
					req := requests[i]
					face, err := r.Face(req.families, req.weight, req.italic, req.size)
					if err != nil {
						t.Fatalf("request %d: %v", i, err)
					}
					if got := face.Font.Subfamily(); got != req.subfamily {
						t.Fatalf("pass %d request %v resolved to %q, want %q", pass, req.families, got, req.subfamily)
					}
					if pass == 0 {
						cold[i] = face
					} else if face != cold[i] {
						t.Fatalf("pass %d request %v did not reuse the cached face", pass, req.families)
					}
				}
			}

			// Layout cache: a fresh prepared document, laid out twice, exercises
			// the layout key and then the warmed registry.
			src := fontRequestDocument(requests, order)
			opts := Options{File: "faces.html", FontDir: fontsDir}
			p, diags, err := prepare(context.Background(), []byte(src), opts)
			if err != nil || p == nil {
				t.Fatalf("prepare: %v\n%s", err, strings.Join(diags.Lines(), "\n"))
			}
			defer func() { _ = p.assets.Close() }()
			for pass := 0; pass < 2; pass++ {
				var layoutDiags diag.List
				faces := layoutPrepared(t, p, opts, &layoutDiags)
				if len(faces) != len(order) {
					t.Fatalf("pass %d: layout painted %d text faces, want %d", pass, len(faces), len(order))
				}
				for pos, i := range order {
					if got := faces[pos].Font.Subfamily(); got != requests[i].subfamily {
						t.Fatalf("pass %d paragraph %d (%v) resolved to %q, want %q",
							pass, pos, requests[i].families, got, requests[i].subfamily)
					}
				}
			}
		})
	}
}

// TestFontRequestPermutationMatrix guards the behavioural invariant behind
// RV4-43: every request signature must resolve to the same face and metrics as
// an isolated cold resolution, whatever order the requests arrive in and
// whether the caches are cold or warm. The matrix spans the exact colliding
// family lists and the weight, slant and size dimensions of the cache keys.
// Reordered documents are different inputs, so their PDFs are compared per
// request, not as whole files.
func TestFontRequestPermutationMatrix(t *testing.T) {
	fontsDir := filepath.Join(repoRoot(), "testdata", "fonts")
	matrix := []fontRequest{
		{families: []string{"a b"}, weight: 400, italic: false, size: 12000, subfamily: "Book"},
		{families: []string{"a", "b"}, weight: 400, italic: false, size: 12000, subfamily: "Bold"},
		{families: []string{"a b"}, weight: 700, italic: false, size: 16000, subfamily: "Book"},
		{families: []string{"a", "b"}, weight: 700, italic: false, size: 16000, subfamily: "Bold"},
		{families: []string{"a b"}, weight: 300, italic: true, size: 10000, subfamily: "Book"},
		{families: []string{"a", "b"}, weight: 300, italic: true, size: 10000, subfamily: "Bold"},
		{families: []string{"a b"}, weight: 700, italic: true, size: 12000, subfamily: "Book"},
		{families: []string{"a", "b"}, weight: 400, italic: true, size: 12000, subfamily: "Bold"},
	}

	// The baseline resolves each request alone in a fresh, cold registry.
	baseline := make([]faceSnapshot, len(matrix))
	for i, req := range matrix {
		r := newTestFontRegistry(t, fontFacesCSS)
		face, err := r.Face(req.families, req.weight, req.italic, req.size)
		if err != nil {
			t.Fatalf("baseline request %d: %v", i, err)
		}
		got := snapshotFace(face)
		if got.subfamily != req.subfamily {
			t.Fatalf("baseline request %d (%v) resolved to %q, want %q", i, req.families, got.subfamily, req.subfamily)
		}
		baseline[i] = got
	}

	permutations := [][]int{
		{0, 1, 2, 3, 4, 5, 6, 7},
		{7, 6, 5, 4, 3, 2, 1, 0},
		{3, 0, 7, 4, 1, 6, 2, 5},
	}

	// The registry cache must return the baseline face for every signature in
	// every order, and a repeated request must reuse the cached face.
	for _, order := range permutations {
		r := newTestFontRegistry(t, fontFacesCSS)
		for _, i := range order {
			req := matrix[i]
			face, err := r.Face(req.families, req.weight, req.italic, req.size)
			if err != nil {
				t.Fatalf("registry order %v request %d: %v", order, i, err)
			}
			if got := snapshotFace(face); got != baseline[i] {
				t.Fatalf("registry order %v request %d: got %+v, want %+v", order, i, got, baseline[i])
			}
			again, err := r.Face(req.families, req.weight, req.italic, req.size)
			if err != nil {
				t.Fatalf("warm request %d: %v", i, err)
			}
			if again != face {
				t.Fatalf("warm request %d did not reuse the cached face", i)
			}
		}
	}

	// Layout assigns the baseline face to every labelled run, in each order.
	for _, order := range permutations {
		src := fontRequestDocument(matrix, order)
		faces := layoutFaces(t, src, Options{File: "matrix.html", FontDir: fontsDir})
		if len(faces) != len(order) {
			t.Fatalf("layout order %v painted %d text faces, want %d", order, len(faces), len(order))
		}
		for pos, i := range order {
			if got := snapshotFace(faces[pos]); got != baseline[i] {
				t.Fatalf("layout order %v paragraph %d: got %+v, want %+v", order, pos, got, baseline[i])
			}
		}
	}

	// Cache warming cannot change a request's result: two layout passes over
	// one prepared document agree with the baseline.
	src := fontRequestDocument(matrix, permutations[0])
	opts := Options{File: "matrix.html", FontDir: fontsDir}
	p, diags, err := prepare(context.Background(), []byte(src), opts)
	if err != nil || p == nil {
		t.Fatalf("prepare: %v\n%s", err, strings.Join(diags.Lines(), "\n"))
	}
	defer func() { _ = p.assets.Close() }()
	for pass := 0; pass < 2; pass++ {
		var layoutDiags diag.List
		faces := layoutPrepared(t, p, opts, &layoutDiags)
		if len(faces) != len(matrix) {
			t.Fatalf("warm pass %d painted %d text faces, want %d", pass, len(faces), len(matrix))
		}
		for i := range matrix {
			if got := snapshotFace(faces[i]); got != baseline[i] {
				t.Fatalf("warm pass %d request %d: got %+v, want %+v", pass, i, got, baseline[i])
			}
		}
	}

	// Repeated identical documents stay byte identical. Reordered documents
	// are different inputs and are compared per request above.
	first, err := Convert(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	second, err := Convert(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatalf("Convert again: %v", err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("two identical documents produced different PDFs")
	}
}

// TestFallbackFamilyIsCharged guards RV4-44: parsing an explicit fallback
// family spends the same budget as an @font-face file, so an exhausted budget
// stops the fallback load instead of parsing it for free.
func TestFallbackFamilyIsCharged(t *testing.T) {
	res, err := Convert(context.Background(), []byte("<p>x</p>"), Options{
		File:          "fallback.html",
		FallbackFonts: []string{testFontPath(t)},
		Limits:        Limits{MaxOperations: 50},
	})
	if !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("want resource/limit, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}
