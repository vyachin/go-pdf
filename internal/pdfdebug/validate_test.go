package pdfdebug

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// samplePDF builds the smallest complete PDF the writer produces: one page with
// one fill operation.
func samplePDF(t *testing.T) []byte {
	t.Helper()
	// The writer's output is not imported here on purpose: the validator must
	// work on bytes, not on the writer's bookkeeping.
	content := "q 1 0 0 rg 10 10 100 100 re f Q\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.276 841.89] /Contents 4 0 R >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects)+2)
	for i, body := range objects {
		offsets[i+1] = b.Len()
		b.WriteString(strconv.Itoa(i+1) + " 0 obj\n" + body + "\nendobj\n")
	}
	offsets[4] = b.Len()
	b.WriteString("4 0 obj\n<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream\nendobj\n")
	xref := b.Len()
	b.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		b.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")
	return b.Bytes()
}

func pad10(v int) string {
	s := strconv.Itoa(v)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// TestValidateAcceptsAValidPDF pins the baseline: a well-formed file passes.
func TestValidateAcceptsAValidPDF(t *testing.T) {
	if err := Validate(samplePDF(t)); err != nil {
		t.Fatalf("a valid file was rejected: %v", err)
	}
}

// TestValidateCatchesAWrongLength guards RV4-30 and IMP-08: a PDF with every
// /Length off by seven used to extract the same text as a valid file.
func TestValidateCatchesAWrongLength(t *testing.T) {
	data := samplePDF(t)
	broken := regexp.MustCompile(`/Length (\d+)`).ReplaceAllFunc(data, func(m []byte) []byte {
		n, _ := strconv.Atoi(string(m[len("/Length "):]))
		return []byte("/Length " + strconv.Itoa(n+7))
	})
	if err := Validate(broken); err == nil || !strings.Contains(err.Error(), "declares") {
		t.Fatalf("a wrong /Length returned %v", err)
	}
}

// TestValidateCatchesAShiftedXref guards the cross-reference half: a table
// whose offsets are one byte off must fail.
func TestValidateCatchesAShiftedXref(t *testing.T) {
	data := samplePDF(t)
	// Shift every in-use entry by one.
	shifted := regexp.MustCompile(`\n(\d{10}) 00000 n `).ReplaceAllFunc(data, func(m []byte) []byte {
		n, _ := strconv.Atoi(string(m[1:11]))
		return []byte("\n" + pad10(n+1) + " 00000 n ")
	})
	if err := Validate(shifted); err == nil || !strings.Contains(err.Error(), "points at") {
		t.Fatalf("a shifted cross-reference table returned %v", err)
	}
	// A startxref one past the table fails too.
	broken := regexp.MustCompile(`startxref\n(\d+)`).ReplaceAllFunc(data, func(m []byte) []byte {
		n, _ := strconv.Atoi(string(m[len("startxref\n"):]))
		return []byte("startxref\n" + strconv.Itoa(n+1))
	})
	if err := Validate(broken); err == nil {
		t.Fatal("a shifted startxref was accepted")
	}
}

// completePDF wraps object bodies into a PDF with a cross-reference table, so a
// test can exercise the extractor on a structurally valid file.
func completePDF(objects ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, body := range objects {
		offsets[i+1] = b.Len()
		b.WriteString(strconv.Itoa(i+1) + " 0 obj\n" + body + "\nendobj\n")
	}
	xref := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(len(objects)+1) + "\n0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		b.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size " + strconv.Itoa(len(objects)+1) + " /Root 1 0 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")
	return b.Bytes()
}
