package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
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

// allocBytes returns the bytes a function allocates on the Go heap. It is the
// quantity a buffered result inflates: the complete PDF is grown in one buffer,
// while a streamed document writes one object at a time.
func allocBytes(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// liveHeap returns the heap retained by a result after the garbage collector
// has run. A streamed Result carries no PDF, so this is the memory the caller
// keeps, not the transient page the serialiser writes.
func liveHeap(fn func() any) int64 {
	debug.FreeOSMemory()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	held := fn()
	debug.FreeOSMemory()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(held)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc)
}

// BenchmarkStreaming compares Convert with ConvertTo to io.Discard over a
// many-page document. It asserts the two memory claims of the change: the
// streamed run allocates at least one PDF size less than the buffered one, and
// the heap retained after a streamed conversion does not include the assembled
// PDF and does not grow with the number of pages.
func BenchmarkStreaming(b *testing.B) {
	fontPath := testFontPath(b)
	opts := Options{FallbackFont: fontPath, File: "bench.html"}
	small := benchDoc(20)
	large := benchDoc(60)

	// Warm up the lazily initialised parts of the pipeline before measuring.
	warmSmall, err := Convert(context.Background(), small, opts)
	if err != nil {
		b.Fatal(err)
	}
	warmLarge, err := Convert(context.Background(), large, opts)
	if err != nil {
		b.Fatal(err)
	}

	convertSmall := allocBytes(func() {
		if _, err := Convert(context.Background(), small, opts); err != nil {
			panic(err)
		}
	})
	streamSmall := allocBytes(func() {
		if _, err := ConvertTo(context.Background(), io.Discard, small, opts); err != nil {
			panic(err)
		}
	})
	convertLarge := allocBytes(func() {
		if _, err := Convert(context.Background(), large, opts); err != nil {
			panic(err)
		}
	})
	streamLarge := allocBytes(func() {
		if _, err := ConvertTo(context.Background(), io.Discard, large, opts); err != nil {
			panic(err)
		}
	})

	smallPDF := uint64(len(warmSmall.PDF))
	largePDF := uint64(len(warmLarge.PDF))
	b.ReportMetric(float64(convertLarge), "buffered-heap-B")
	b.ReportMetric(float64(streamLarge), "streamed-heap-B")
	b.ReportMetric(float64(largePDF), "pdf-B")

	if streamSmall+smallPDF > convertSmall {
		b.Fatalf("streaming 20 pages allocated %d bytes, buffering %d: the stream must save at least the %d-byte PDF",
			streamSmall, convertSmall, smallPDF)
	}
	if streamLarge+largePDF > convertLarge {
		b.Fatalf("streaming 60 pages allocated %d bytes, buffering %d: the stream must save at least the %d-byte PDF",
			streamLarge, convertLarge, largePDF)
	}
	convertLive := liveHeap(func() any {
		res, err := Convert(context.Background(), large, opts)
		if err != nil {
			panic(err)
		}
		return res
	})
	streamLive := liveHeap(func() any {
		res, err := ConvertTo(context.Background(), io.Discard, large, opts)
		if err != nil {
			panic(err)
		}
		return res
	})
	streamLiveSmall := liveHeap(func() any {
		res, err := ConvertTo(context.Background(), io.Discard, small, opts)
		if err != nil {
			panic(err)
		}
		return res
	})
	b.Logf("allocated: convert=%d/%d stream=%d/%d pdf=%d/%d; retained: convert=%d stream=%d (small stream=%d)",
		convertSmall, convertLarge, streamSmall, streamLarge, smallPDF, largePDF,
		convertLive, streamLive, streamLiveSmall)

	// The buffered result keeps the complete PDF alive; the streamed one keeps
	// only the diagnostics, so the difference must cover the PDF.
	if convertLive-streamLive < int64(largePDF) {
		b.Fatalf("the buffered result retains %d bytes and the streamed one %d: the difference does not cover the %d-byte PDF",
			convertLive, streamLive, largePDF)
	}
	// The streamed result does not retain the assembled PDF at all.
	if streamLive >= int64(largePDF) {
		b.Fatalf("the streamed result retains %d bytes, as much as the %d-byte PDF", streamLive, largePDF)
	}
	// One page's output plus an allowance for runtime heap bookkeeping is the
	// whole bound: streaming a larger document must not retain a growing copy.
	pages := int64(strings.Count(string(warmLarge.PDF), "/Type /Page "))
	if pages == 0 {
		b.Fatal("the large benchmark document has no pages")
	}
	onePage := int64(largePDF) / pages
	if streamLive > streamLiveSmall+onePage+(256<<10) {
		b.Fatalf("streaming %d pages retains %d bytes, a smaller document retains %d and one page carries %d bytes",
			pages, streamLive, streamLiveSmall, onePage)
	}

	b.Run("Convert", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Convert(context.Background(), large, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ConvertTo", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := ConvertTo(context.Background(), io.Discard, large, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
}
