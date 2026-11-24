package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

// TestProfileDocMatchesParser guards RV2-25: docs/profile.md promised
// display: inline-block, which the validator rejects. README calls the table the
// complete list, so every value it lists must be accepted by the parser.
func TestProfileDocMatchesParser(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "profile.md"))
	if err != nil {
		t.Skipf("docs/profile.md is unavailable: %v", err)
	}
	values := docRowValues(t, string(data), "display")
	if len(values) == 0 {
		t.Fatal("the display row of docs/profile.md lists no values")
	}
	for _, value := range values {
		var st css.Style
		err := css.ApplyDecl(&st, "display", css.Tokenize([]byte(value)), 12000)
		if err != nil {
			t.Errorf("docs/profile.md lists display: %s, but the parser rejects it: %v", value, err)
		}
	}
	if strings.Contains(string(data), "`inline-block`") {
		t.Error("docs/profile.md still lists inline-block")
	}
}

// docRowValues returns the backticked values of the property table row.
func docRowValues(t *testing.T, doc, property string) []string {
	t.Helper()
	prefix := "| `" + property + "` |"
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			return nil
		}
		var out []string
		for _, part := range strings.Split(cells[3], "`") {
			part = strings.Trim(part, " ,")
			if part == "" {
				continue
			}
			out = append(out, part)
		}
		return out
	}
	return nil
}
