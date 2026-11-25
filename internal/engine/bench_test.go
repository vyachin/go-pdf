package engine

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func benchDoc(pages int) []byte {
	var b strings.Builder
	b.WriteString(`<style>@page { size: A4; margin: 20mm }
		body { font-family: "PT Serif"; font-size: 12pt }
		p { margin: 0 0 3mm 0 }
	</style>`)
	sentence := "Настоящий документ подтверждает выполнение работ по договору и является основанием для оплаты. "
	for i := 0; i < pages*12; i++ {
		fmt.Fprintf(&b, "<p>%d. %s</p>\n", i+1, strings.Repeat(sentence, 2))
	}
	return []byte(b.String())
}

func BenchmarkConvertOnePage(b *testing.B) {
	fontPath := testFontPath(b)
	src := []byte(`<style>p { font-size: 12pt }</style><p>Одна страница текста для проверки производительности.</p>`)
	opts := Options{FallbackFont: fontPath, File: "bench.html"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Convert(context.Background(), src, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvertFiftyPages(b *testing.B) {
	fontPath := testFontPath(b)
	src := benchDoc(50)
	opts := Options{FallbackFont: fontPath, File: "bench.html"}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		res, err := Convert(context.Background(), src, opts)
		if err != nil {
			b.Fatal(err)
		}
		if len(res.PDF) == 0 {
			b.Fatal("no output")
		}
	}
}

func TestPageCountOfBenchmarkDocument(t *testing.T) {
	// The benchmark document runs only on request (PDF_PERF=1), with the
	// performance report, never as part of the ordinary or race test suite.
	if os.Getenv("PDF_PERF") != "1" {
		t.Skip("performance check; set PDF_PERF=1 to run")
	}
	fontPath := testFontPath(t)
	res, err := Convert(context.Background(), benchDoc(50), Options{FallbackFont: fontPath, File: "bench.html"})
	if err != nil {
		t.Fatal(err)
	}
	pages := strings.Count(string(res.PDF), "/Type /Page ")
	if pages < 40 {
		t.Fatalf("got %d pages, want about 50", pages)
	}
}

func TestPerformanceReport(t *testing.T) {
	// Timing and allocation numbers depend on the machine, so this report runs
	// only on request (PDF_PERF=1), never as part of the ordinary or race test
	// suite, and it asserts no threshold.
	if os.Getenv("PDF_PERF") != "1" {
		t.Skip("performance report; set PDF_PERF=1 to run")
	}
	fontPath := testFontPath(t)
	one := []byte(`<p>Одна страница.</p>`)
	opts := Options{FallbackFont: fontPath, File: "bench.html"}

	if _, err := Convert(context.Background(), one, opts); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	res, err := Convert(context.Background(), benchDoc(50), opts)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("50 pages: %v, %.1f MB allocated, %d bytes of PDF", elapsed, float64(allocated)/(1<<20), len(res.PDF))
}
