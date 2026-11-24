package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPHPUnescape(t *testing.T) {
	got := phpUnescape(`a\nb\tc\'d\\e`)
	want := "a\nb\tc'd\\e"
	if got != want {
		t.Fatalf("phpUnescape = %q, want %q", got, want)
	}
}

func TestKebab(t *testing.T) {
	cases := map[string]string{
		"BasicHtml":             "basic-html",
		"Rtl":                   "rtl",
		"BackgroundsAndBorders": "backgrounds-and-borders",
		"":                      "",
	}
	for in, want := range cases {
		if got := kebab(in); got != want {
			t.Fatalf("kebab(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMethodBefore(t *testing.T) {
	src := "class X {\n  public function testOne() {\n    $a = 1;\n  }\n  public function testTwo() {\n    $b = 2;\n  }\n}"
	pos := strings.Index(src, "$b")
	if got, want := methodBefore(src, pos), "testTwo"; got != want {
		t.Fatalf("methodBefore = %q, want %q", got, want)
	}
	if got := methodBefore(src, 0); got != "" {
		t.Fatalf("methodBefore at the start = %q, want empty", got)
	}
}

func TestImportIssueTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Issue42Test.php")
	src := `<?php
class Issue42Test {
	public function testSimple() {
		$mpdf->WriteHTML('<p class="a">Привет &amp; пока</p>');
	}
	public function testHeredoc() {
		$html = <<<HTML
<p>Строка 1</p>
<p>Строка 2</p>
HTML;
		$mpdf->WriteHTML($html);
	}
	public function testDynamic() {
		$mpdf->WriteHTML($this->build());
	}
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := importIssueTest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2: %+v", len(cases), cases)
	}
	if cases[0].HTML != `<p class="a">Привет &amp; пока</p>` {
		t.Fatalf("first case = %q", cases[0].HTML)
	}
	if !strings.Contains(cases[1].HTML, "Строка 1") || !strings.Contains(cases[1].HTML, "Строка 2") {
		t.Fatalf("heredoc case = %q", cases[1].HTML)
	}
	for _, c := range cases {
		if c.Kind != "issue" || c.Source != "tests/Issues/Issue42Test.php" {
			t.Fatalf("unexpected metadata: %+v", c)
		}
	}
}

func TestImportSnapshotTestStatic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BasicHtmlSnapshotTest.php")
	src := `<?php
class BasicHtmlSnapshotTest {
	public function generatePdf() {
		ob_start();
		?>
		<h1>Заголовок</h1>
		<p>Текст</p>
		<?php
		$html = ob_get_clean();
		$this->mpdf->WriteHTML($html);
	}
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := importSnapshotTest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Name != "basic-html-snapshot" {
		t.Fatalf("unexpected cases: %+v", cases)
	}
	if !strings.Contains(cases[0].HTML, "<h1>Заголовок</h1>") {
		t.Fatalf("HTML = %q", cases[0].HTML)
	}
}

func TestImportSnapshotTestDynamic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ColumnsSnapshotTest.php")
	src := `<?php
class ColumnsSnapshotTest {
	public function generatePdf() {
		$this->mpdf->WriteHTML('<p>a</p>');
	}
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := importSnapshotTest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Skip == "" {
		t.Fatalf("a PHP-built document must be skipped: %+v", cases)
	}
}

func TestImportCorpusRejectsEmptySource(t *testing.T) {
	if _, err := importCorpus(t.TempDir()); err == nil {
		t.Fatal("expected an error when no issue tests are found")
	}
}
