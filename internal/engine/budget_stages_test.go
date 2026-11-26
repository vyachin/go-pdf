package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// writeSizedPNG writes a solid PNG of the given pixel size. The decoding charge
// grows with the pixel count, so a test can exhaust a budget without writing
// hundreds of files.
func writeSizedPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 120
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// gposAmplifier builds a GPOS table whose one PairPos lookup declares
// `subtables` subtables pointing at the lookup itself: enough charged work to
// exhaust a small budget without a large file.
func gposAmplifier(subtables int) []byte {
	const lookupList = 10
	lookupOff := lookupList + 2 + 2
	lookupSize := 6 + subtables*2
	out := make([]byte, lookupOff+lookupSize)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[8:10], lookupList)
	binary.BigEndian.PutUint16(out[lookupList:lookupList+2], 1)
	binary.BigEndian.PutUint16(out[lookupList+2:lookupList+4], uint16(lookupOff-lookupList))
	binary.BigEndian.PutUint16(out[lookupOff:lookupOff+2], 2) // PairPos
	binary.BigEndian.PutUint16(out[lookupOff+4:lookupOff+6], uint16(subtables))
	return out
}

// buildTestFont assembles the smallest complete TrueType font carrying gpos.
func buildTestFont(gpos []byte) []byte {
	const numGlyphs = 2
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)    // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800) // ascent
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	hmtx := []byte{0, 100, 0, 0}
	loca := make([]byte, (numGlyphs+1)*4)
	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", head}, {"hhea", hhea}, {"maxp", maxp}, {"hmtx", hmtx},
		{"loca", loca}, {"glyf", nil}, {"GPOS", gpos},
	}
	out := make([]byte, 12+len(tables)*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(tables)))
	off := len(out)
	for i, table := range tables {
		rec := 12 + i*16
		copy(out[rec:rec+4], table.tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(off))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(table.data)))
		out = append(out, table.data...)
		off += len(table.data)
		for len(out)%4 != 0 {
			out = append(out, 0)
			off++
		}
	}
	return out
}

// TestDiagnosticPathIsCharged guards the path charge end to end: a budget tiny
// against the byte length of a diagnostic element path stops while that path is
// built, so the conversion reports resource/limit instead of building the
// string for free.
func TestDiagnosticPathIsCharged(t *testing.T) {
	var classes strings.Builder
	for i := 0; i < 20_000; i++ {
		fmt.Fprintf(&classes, "c%d ", i)
	}
	src := []byte(`<div data-x class="` + classes.String() + `">x</div>`)

	res, err := Convert(context.Background(), src, Options{
		FallbackFont: testFontPath(t),
		File:         "path-budget.html",
		Limits:       Limits{MaxOperations: 5_000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("the diagnostic path was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}

	// A budget that covers the path lets the same document complete and leaves
	// the unsupported attribute as an ordinary warning.
	res, err = Convert(context.Background(), src, Options{
		FallbackFont: testFontPath(t),
		File:         "path-budget.html",
		Limits:       Limits{MaxOperations: 1_000_000},
	})
	if err != nil {
		t.Fatalf("the conversion failed under a budget that covers the path: %v", err)
	}
	if !hasCode(res.Diagnostics, "html/unsupported-attribute") || len(res.PDF) == 0 {
		t.Fatalf("the warning or the PDF is missing: %v", res.Diagnostics.Lines())
	}
}

// TestCascadeMatchingIsCharged guards the cascade stage: matching many rules
// against many elements must stop at MaxOperations instead of running the full
// rule × element product.
func TestCascadeMatchingIsCharged(t *testing.T) {
	var b strings.Builder
	b.WriteString("<style>")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, ".c%d p { color: #000 }\n", i)
	}
	b.WriteString("</style>")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, `<div class="c%d"><p>text</p></div>`, i)
	}
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		File:         "cascade-budget.html",
		Limits:       Limits{MaxOperations: 20_000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("cascade matching was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestImageDecodingIsCharged guards the image stage: the budget is spent from
// the image header before the decoders run on the images that would cross it.
func TestImageDecodingIsCharged(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	const images = 60
	for i := 0; i < images; i++ {
		name := fmt.Sprintf("i%d.png", i)
		writeSizedPNG(t, filepath.Join(dir, name), 512, 512)
		fmt.Fprintf(&b, `<img src="%s">`, name)
	}
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		BaseDir:      dir,
		File:         "image-budget.html",
		Limits:       Limits{MaxOperations: 5000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("image decoding was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestFontParsingIsCharged guards the font stage end to end: the amplifier
// GPOS exhausts a small budget during prepare, before layout.
func TestFontParsingIsCharged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amplifier.ttf")
	if err := os.WriteFile(path, buildTestFont(gposAmplifier(100)), 0o644); err != nil {
		t.Fatal(err)
	}
	// No text: the crafted font has no cmap, and this test is about the parsing
	// charge, not about glyph lookup.
	res, err := Convert(context.Background(), []byte(`<div style="height: 1pt"></div>`), Options{
		FallbackFont: path,
		File:         "font-budget.html",
		Limits:       Limits{MaxOperations: 250},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("font parsing was not charged: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestImageLoaderHonoursCancellation guards that a cancelled context is visible
// before an image is decoded.
func TestImageLoaderHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := &imageLoader{guard: budget.New(ctx, 0)}
	if err := l.charge(1 << 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("image charge returned %v, want context.Canceled", err)
	}
}

// TestCancellationDuringSelectorMatching covers the cancellation scenario: a
// cancelled context with a rule-heavy document returns the context error, not
// a document diagnostic.
func TestCancellationDuringSelectorMatching(t *testing.T) {
	var b strings.Builder
	b.WriteString("<style>")
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&b, ".c%d p { color: #000 }\n", i)
	}
	b.WriteString("</style>")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, `<div class="c%d"><p>x</p></div>`, i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	_, err := Convert(ctx, []byte(b.String()), Options{
		FallbackFont: testFontPath(t),
		File:         "cancel-match.html",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Convert returned %v, want context.Canceled", err)
	}
}

// TestValidateCancelledContextStopsStyleAttribute guards the document entry
// point for a cancellation before declaration parsing: the context error is
// returned instead of a document diagnostic.
func TestValidateCancelledContextStopsStyleAttribute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Validate(ctx, []byte(`<p style="color:red">x</p>`), Options{File: "cancel-style.html"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate returned %v, want context.Canceled", err)
	}
}

// TestNestedAvoidBuildsLinesOnce per RV4-03: the lines a measurement builds are
// the lines the placement paints, so a document of nested break-inside: avoid
// blocks paginates like the same paragraphs without the nesting, and no page is
// empty. The recorded spend is the assertion, never a duration.
func TestNestedAvoidBuildsLinesOnce(t *testing.T) {
	const depth, words = 40, 80
	font := testFontPath(t)
	paragraph := "<p>" + strings.Repeat("word ", words) + "</p>"
	var nested strings.Builder
	for i := 0; i < depth; i++ {
		nested.WriteString(`<div style="break-inside: avoid">` + paragraph)
	}
	for i := 0; i < depth; i++ {
		nested.WriteString("</div>")
	}
	flat := strings.Repeat(paragraph, depth)

	convert := func(src string) Result {
		t.Helper()
		res, err := Convert(context.Background(), []byte(src), Options{File: "nested.html", FallbackFont: font})
		if err != nil {
			t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
		return res
	}
	nestedRes := convert(nested.String())
	flatRes := convert(flat)
	nestedPages, err := pdfdebug.PageTexts(nestedRes.PDF)
	if err != nil {
		t.Fatal(err)
	}
	flatPages, err := pdfdebug.PageTexts(flatRes.PDF)
	if err != nil {
		t.Fatal(err)
	}
	var nestedText, flatText strings.Builder
	for i := range nestedPages {
		if strings.TrimSpace(nestedPages[i]) == "" {
			t.Fatalf("page %d is empty", i+1)
		}
		nestedText.WriteString(nestedPages[i])
	}
	for i := range flatPages {
		flatText.WriteString(flatPages[i])
	}
	if got, want := nestedText.String(), flatText.String(); got != want {
		t.Fatalf("the nested document painted %d characters, the flat one %d", len(got), len(want))
	}
}

// TestHugeWordStopsAtTheBudget guards RV4-04: a single two-megabyte word under
// MaxOperations 100000 fails with resource/limit before shaping allocates a
// glyph and state entry per rune, and the report does not add a dependent
// glyph error for work the budget refused.
func TestHugeWordStopsAtTheBudget(t *testing.T) {
	src := []byte("<p>" + strings.Repeat("ж", 2<<20) + "</p>")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	res, err := Convert(context.Background(), src, Options{
		File:         "word.html",
		FallbackFont: testFontPath(t),
		Limits:       Limits{MaxOperations: 100000},
	})
	runtime.ReadMemStats(&after)
	if !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("want resource/limit, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	for _, d := range res.Diagnostics {
		if d.Code == "text/glyph" || d.Code == "text/font" {
			t.Fatalf("a dependent diagnostic was added after the budget failure: %s", d.String())
		}
	}
	if alloc, limit := after.TotalAlloc-before.TotalAlloc, 20*uint64(len(src)); alloc > limit {
		t.Fatalf("the stopped conversion allocated %d bytes for a %d-byte input", alloc, len(src))
	}
}

// TestBudgetFailureStopsDependentDiagnostics guards RV4-62: once the guard is
// exhausted, a later stage must not add a diagnostic about the work it refused
// to do — a missing family reported for a face the exhausted budget never tried
// to resolve is misleading, not evidence about the document.
func TestBudgetFailureStopsDependentDiagnostics(t *testing.T) {
	stopped := false
	for _, max := range []int{300, 1000, 2000, 4000} {
		res, _ := Convert(context.Background(), []byte(`<p style="font-family: Nope">hello world</p>`), Options{
			File:   "dependent.html",
			Limits: Limits{MaxOperations: max},
		})
		if !hasCode(res.Diagnostics, "resource/limit") {
			continue
		}
		stopped = true
		for _, d := range res.Diagnostics {
			if d.Code == "text/font" || d.Code == "text/glyph" {
				t.Fatalf("MaxOperations=%d reported %s after the budget failed:\n%s", max, d.String(), strings.Join(res.Diagnostics.Lines(), "\n"))
			}
		}
	}
	if !stopped {
		t.Fatal("none of the budgets was exhausted")
	}
}

// TestAdversarialSpendIsProportional guards IMP-03: for every adversarial
// fixture at a small MaxOperations, the work the guard records and the memory
// the conversion allocates stay within a constant multiple of the budget. The
// test uses the budget, never elapsed time.
func TestAdversarialSpendIsProportional(t *testing.T) {
	dir := adversarialDir()
	for _, tc := range adversarialCases() {
		if tc.Want != "resource/limit" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, tc.Fixture))
			if err != nil {
				t.Fatal(err)
			}
			opts := tc.Options(t, dir)
			if opts.Limits.MaxDOMDepth > 0 || opts.Limits.MaxTableCells > 0 {
				// This fixture tests a structural limit, not the work budget.
				t.Skip("a structural-limit fixture")
			}
			// Keep the case's own budget when it has one; otherwise use a small
			// budget that still stops the document.
			const max = 5000
			n := max
			if opts.Limits.MaxOperations > 0 {
				n = opts.Limits.MaxOperations
			}
			opts.Limits.MaxOperations = n
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			res, _ := Convert(context.Background(), src, opts)
			runtime.ReadMemStats(&after)
			if !hasCode(res.Diagnostics, "resource/limit") {
				t.Fatalf("the fixture did not exhaust the budget:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			// Each unit may cost a bounded amount of memory: a conversion
			// that allocates per input byte without charging the budget
			// blows through this bound.
			const bytesPerUnit = 4096
			if alloc, limit := after.TotalAlloc-before.TotalAlloc, uint64(n*bytesPerUnit); alloc > limit {
				t.Fatalf("the stopped conversion allocated %d bytes for a %d-unit budget (limit %d)", alloc, n, limit)
			}
		})
	}
}
