package engine

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden corpus files")

func TestCorpus(t *testing.T) {
	fontPath := testFontPath(t)
	corpus := []struct {
		file  string
		marks []string
	}{
		{"invoice.html", []string{"Счёт", "Итого", "НДС", "534"}},
		{"act.html", []string{"Акт", "Разработка", "Исполнитель"}},
		{"contract.html", []string{"Договор", "Предмет договора", "Ответственность"}},
		{"bigtable.html", []string{"Отчёт", "Показы", "Итого", "2 690 000"}},
	}
	for _, tc := range corpus {
		t.Run(tc.file, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", "..", "testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			opts := Options{FallbackFont: fontPath, BaseDir: filepath.Join("..", "..", "testdata"), File: tc.file}
			res, err := Convert(context.Background(), src, opts)
			if err != nil {
				t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			validateWithPDFCPU(t, res.PDF)
			if res.Diagnostics.HasErrors() {
				t.Fatalf("unexpected errors: %v", res.Diagnostics.Lines())
			}
			text, err := extractText(res.PDF)
			if err != nil {
				t.Fatal(err)
			}
			for _, mark := range tc.marks {
				if !strings.Contains(text, mark) {
					t.Fatalf("text does not contain %q", mark)
				}
			}
			// Repeated conversion must be byte identical.
			again, err := Convert(context.Background(), src, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(res.PDF, again.PDF) {
				t.Fatal("two conversions of the same template differ")
			}
			checkGolden(t, strings.TrimSuffix(tc.file, ".html")+".txt", text)
		})
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no golden file %s; run go test ./internal/engine -update", name)
	}
	if strings.TrimSpace(string(want)) != strings.TrimSpace(got) {
		t.Fatalf("text differs from %s\n got: %q\nwant: %q", name, truncate(got, 400), truncate(string(want), 400))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
