package pdfdebug

import (
	"bytes"
	"testing"
)

// TestPagesFollowThePageTree guards the page tree: an object whose stream
// contains "TJ" but is not a page must not add a phantom page.
func TestPagesFollowThePageTree(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" +
		"1 0 obj\n<< /Type /Pages /Kids [2 0 R] /Count 1 >>\nendobj\n" +
		"2 0 obj\n<< /Type /Page /Parent 1 0 R /Contents 3 0 R >>\nendobj\n" +
		"3 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n" +
		"4 0 obj\n<< /Length 2 >>\nstream\nTJ\nendstream\nendobj\n")
	pages, err := Pages(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("Pages = %d, want the one page in the tree", len(pages))
	}
	streams := ContentStreams(pdf)
	if len(streams) != 1 {
		t.Fatalf("ContentStreams = %d, want the one page stream", len(streams))
	}
}

// TestContentStreamOrderIsStable guards the order: it comes from the page
// tree's Kids array, not from map iteration.
func TestContentStreamOrderIsStable(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" +
		"1 0 obj\n<< /Type /Pages /Kids [2 0 R 5 0 R] /Count 2 >>\nendobj\n" +
		"2 0 obj\n<< /Type /Page /Parent 1 0 R /Contents 3 0 R >>\nendobj\n" +
		"3 0 obj\n<< /Length 4 >>\nstream\n%p1\nendstream\nendobj\n" +
		"4 0 obj\n<< /Length 4 >>\nstream\n%p2\nendstream\nendobj\n" +
		"5 0 obj\n<< /Type /Page /Parent 1 0 R /Contents 4 0 R >>\nendobj\n")
	want := []string{"%p1", "%p2"}
	for i := 0; i < 100; i++ {
		streams := ContentStreams(pdf)
		if len(streams) != len(want) {
			t.Fatalf("run %d: %d streams", i, len(streams))
		}
		for j := range want {
			if !bytes.Contains(streams[j], []byte(want[j])) {
				t.Fatalf("run %d: stream %d = %q, want %q", i, j, streams[j], want[j])
			}
		}
	}
}
