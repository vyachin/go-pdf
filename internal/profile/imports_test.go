package profile

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbiddenImports are the packages validation must not use. Loading and
// decoding belong to internal/stylesheet; a direct dependency here would put a
// file read back into the validator.
var forbiddenImports = []string{
	"github.com/vyachin/go-pdf/internal/assets",
	"github.com/vyachin/go-pdf/internal/fsx",
	"github.com/vyachin/go-pdf/internal/asset",
	"os",
	"path/filepath",
}

// TestValidationHasNoAssetImports guards RV5-50: the profile validates only, so
// its production files must not import an asset or filesystem package. Parsing
// the package's own sources with go/parser is hermetic and names the file that
// reintroduced the dependency; _test.go files are excluded because a test
// legitimately opens fixtures.
func TestValidationHasNoAssetImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		checked++
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: reading import %s: %v", name, spec.Path.Value, err)
			}
			for _, forbidden := range forbiddenImports {
				if path == forbidden || strings.HasPrefix(path, forbidden+"/") {
					t.Errorf("%s imports %q: loading and decoding belong to internal/stylesheet", name, path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no production files were parsed")
	}
}
