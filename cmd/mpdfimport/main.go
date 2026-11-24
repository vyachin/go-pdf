// Command mpdfimport converts the HTML fixtures of the mPDF test suite into a
// portable corpus for this engine.
//
// mPDF's tests are PHPUnit tests: they assert PHP internals (Imagick snapshots,
// reflection, mPDF-specific objects) and cover features outside this engine's
// profile. What is portable is the HTML itself, and that is what this tool
// extracts: every statically analysable WriteHTML() fragment from the issue
// tests and every snapshot document that is written as plain HTML.
//
// Usage:
//
//	git clone --depth 1 https://github.com/mpdf/mpdf /tmp/mpdf
//	go run ./cmd/mpdfimport -src /tmp/mpdf/tests -out testdata/mpdf
//
// The generated corpus is committed, so the test suite does not need network
// access.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Case is one imported document.
type Case struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`           // "issue" or "snapshot"
	Source string `json:"source"`         // path inside the mPDF repository
	HTML   string `json:"html,omitempty"` // document source
	Skip   string `json:"skip,omitempty"` // reason when nothing could be extracted
}

type corpus struct {
	Source string `json:"source"`
	Note   string `json:"note"`
	Cases  []Case `json:"cases"`
}

var (
	writeHTMLRe = regexp.MustCompile(`(?is)WriteHTML\s*\(\s*'((?:[^'\\]|\\.)*)'`)
	heredocHead = regexp.MustCompile(`(?m)<<<\s*'?([A-Za-z_][A-Za-z0-9_]*)'?[ \t]*$`)
	functionRe  = regexp.MustCompile(`(?i)function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	snapshotRe  = regexp.MustCompile(`(?s)ob_start\(\);\s*\?>(.*?)<\?php\s*\$html\s*=\s*ob_get_clean\(\)`)
	phpBlockRe  = regexp.MustCompile(`(?s)<\?(?:php|=).*?\?>`)
)

func main() {
	src := flag.String("src", "", "path to the mPDF tests directory")
	out := flag.String("out", "testdata/mpdf", "output directory")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "mpdfimport: -src is required")
		os.Exit(2)
	}
	c, err := importCorpus(*src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mpdfimport: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mpdfimport: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(*out, "cases.json"), c); err != nil {
		fmt.Fprintf(os.Stderr, "mpdfimport: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*out, "NOTICE.md"), []byte(notice), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "mpdfimport: %v\n", err)
		os.Exit(1)
	}
	stats := map[string]int{}
	for _, cs := range c.Cases {
		if cs.Skip != "" {
			stats[cs.Kind+"/skipped"]++
		} else {
			stats[cs.Kind]++
		}
	}
	fmt.Printf("imported %d cases into %s\n", len(c.Cases), *out)
	for _, kind := range []string{"issue", "snapshot"} {
		fmt.Printf("  %-9s documents: %d, skipped files: %d\n", kind, stats[kind], stats[kind+"/skipped"])
	}
}

func importCorpus(src string) (*corpus, error) {
	c := &corpus{
		Source: "https://github.com/mpdf/mpdf/tree/development/tests",
		Note:   "HTML extracted from the mPDF test suite. The PHP assertions are not portable, so this corpus is a compatibility and robustness suite, not a snapshot of mPDF output.",
	}
	issues, err := filepath.Glob(filepath.Join(src, "Issues", "*.php"))
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("no issue tests found under %s", filepath.Join(src, "Issues"))
	}
	sort.Strings(issues)
	for _, path := range issues {
		cases, err := importIssueTest(path)
		if err != nil {
			return nil, err
		}
		c.Cases = append(c.Cases, cases...)
	}
	snapshots, err := filepath.Glob(filepath.Join(src, "Snapshots", "*Test.php"))
	if err != nil {
		return nil, err
	}
	sort.Strings(snapshots)
	for _, path := range snapshots {
		cases, err := importSnapshotTest(path)
		if err != nil {
			return nil, err
		}
		c.Cases = append(c.Cases, cases...)
	}
	sort.Slice(c.Cases, func(i, j int) bool { return c.Cases[i].Name < c.Cases[j].Name })
	return c, nil
}

func importIssueTest(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(data)
	base := strings.TrimSuffix(filepath.Base(path), ".php")
	rel := "tests/Issues/" + filepath.Base(path)

	type found struct {
		html   string
		method string
		start  int
	}
	var snippets []found
	for _, m := range writeHTMLRe.FindAllStringSubmatchIndex(src, -1) {
		snippets = append(snippets, found{
			html:   phpUnescape(src[m[2]:m[3]]),
			method: methodBefore(src, m[0]),
			start:  m[0],
		})
	}
	for _, h := range extractHeredocs(src) {
		snippets = append(snippets, found{
			html:   h.html,
			method: methodBefore(src, h.start),
			start:  h.start,
		})
	}
	sort.Slice(snippets, func(i, j int) bool { return snippets[i].start < snippets[j].start })

	var out []Case
	seen := map[string]bool{}
	for i, s := range snippets {
		html := strings.TrimSpace(s.html)
		if html == "" {
			continue
		}
		suffix := strings.ToLower(s.method)
		if suffix == "" {
			suffix = fmt.Sprintf("case%d", i+1)
		}
		name := base + "_" + suffix
		if seen[name] {
			name = fmt.Sprintf("%s_%d", name, i+1)
		}
		seen[name] = true
		out = append(out, Case{Name: name, Kind: "issue", Source: rel, HTML: html})
	}
	if len(out) == 0 {
		out = append(out, Case{
			Name:   base,
			Kind:   "issue",
			Source: rel,
			Skip:   "HTML is built dynamically in PHP",
		})
	}
	return out, nil
}

func importSnapshotTest(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(data)
	rel := "tests/Snapshots/" + filepath.Base(path)
	name := kebab(strings.TrimSuffix(filepath.Base(path), "Test.php"))
	m := snapshotRe.FindStringSubmatch(src)
	if m == nil {
		return []Case{{Name: name, Kind: "snapshot", Source: rel, Skip: "document is assembled with PHP variables"}}, nil
	}
	html := m[1]
	if phpBlockRe.MatchString(html) {
		return []Case{{Name: name, Kind: "snapshot", Source: rel, Skip: "document contains PHP control flow"}}, nil
	}
	return []Case{{Name: name, Kind: "snapshot", Source: rel, HTML: strings.TrimSpace(html)}}, nil
}

// extractHeredocs finds PHP heredoc bodies. RE2 has no backreferences, so the
// closing delimiter is matched by scanning lines.
func extractHeredocs(src string) []struct {
	html  string
	start int
} {
	var out []struct {
		html  string
		start int
	}
	for _, m := range heredocHead.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		bodyStart := m[1]
		// Move past the rest of the opening line.
		if nl := strings.IndexByte(src[bodyStart:], '\n'); nl >= 0 {
			bodyStart += nl + 1
		} else {
			continue
		}
		rest := src[bodyStart:]
		end := -1
		lines := strings.Split(rest, "\n")
		offset := 0
		for _, line := range lines {
			trimmed := strings.TrimSuffix(strings.TrimSpace(line), ";")
			if trimmed == name {
				end = offset
				break
			}
			offset += len(line) + 1
		}
		if end < 0 {
			continue
		}
		body := strings.TrimSuffix(rest[:end], "\n")
		out = append(out, struct {
			html  string
			start int
		}{html: body, start: m[0]})
	}
	return out
}

func methodBefore(src string, pos int) string {
	matches := functionRe.FindAllStringSubmatchIndex(src[:pos], -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	return src[last[2]:last[3]]
}

// phpUnescape decodes the escapes that appear in single quoted PHP strings.
func phpUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// kebab converts "BasicHtml" to "basic-html".
func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

const notice = `# Источник корпуса

HTML-фрагменты в этом каталоге взяты из тестов проекта mPDF:

- https://github.com/mpdf/mpdf/tree/development/tests

mPDF распространяется под лицензией GPL-2.0. Фрагменты используются как
тестовые данные (входные документы), а не как код проекта. Тестовый
harness в ` + "`internal/mpdfcompat`" + ` написан с нуля и не является
производной от PHP-кода mPDF.

Если проект будет распространяться под несовместимой лицензией, этот
каталог нужно либо исключить из поставки, либо заменить фрагменты на
собственные эквиваленты.
`
