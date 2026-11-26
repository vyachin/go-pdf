package pdfgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestExportedDocs enforces the rule the package states: every exported
// identifier and the package itself carries a doc comment. It reads the source
// rather than the compiled package, so a missing comment fails the suite.
func TestExportedDocs(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package files: %v", err)
	}
	fset := token.NewFileSet()
	packageDoc := false
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if hasDoc(file.Doc) {
			packageDoc = true
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if !hasDoc(d.Doc, s.Doc) {
							undocumented(t, fset, "type "+s.Name.Name, s.Pos())
						}
						structType, ok := s.Type.(*ast.StructType)
						if !ok {
							continue
						}
						for _, field := range structType.Fields.List {
							for _, name := range field.Names {
								if name.IsExported() && !hasDoc(field.Doc, field.Comment) {
									undocumented(t, fset, "field "+s.Name.Name+"."+name.Name, name.Pos())
								}
							}
						}
					case *ast.ValueSpec:
						if len(s.Names) == 0 {
							continue
						}
						kind := "const"
						if d.Tok == token.VAR {
							kind = "var"
						}
						if !hasDoc(d.Doc, s.Doc) {
							undocumented(t, fset, kind+" "+s.Names[0].Name, s.Pos())
						}
					}
				}
			case *ast.FuncDecl:
				if d.Name.IsExported() && !hasDoc(d.Doc) {
					undocumented(t, fset, "function "+d.Name.Name, d.Pos())
				}
			}
		}
	}
	if !packageDoc {
		t.Error("the package has no doc comment")
	}
}

// TestRenderPNGContractDocs guards RV5-51 at the source level: the public
// field states its exact preview contract and its deprecation, and the mirrored
// engine field states the same contract without claiming a command-line caller.
// The doc is read from the field itself, so a phrase that survives elsewhere in
// a file does not satisfy the test.
func TestRenderPNGContractDocs(t *testing.T) {
	checks := []struct {
		path  string
		want  []string
		local bool
	}{
		{"pdfgen.go", []string{"first page", "96 dpi", "MaxRasterPixels", "Deprecated:"}, true},
		{filepath.Join("..", "internal", "engine", "engine.go"), []string{"debug preview", "first page", "96 dpi", "MaxRasterPixels"}, false},
	}
	for _, check := range checks {
		doc := structFieldDoc(t, check.path, "Options", "RenderPNG")
		for _, phrase := range check.want {
			if !strings.Contains(doc, phrase) {
				t.Errorf("%s: Options.RenderPNG does not say %q:\n%s", check.path, phrase, doc)
			}
		}
		if !check.local {
			if regexp.MustCompile(`CLI\s+debug`).MatchString(doc) {
				t.Errorf("%s: Options.RenderPNG still claims a command-line flag drives the preview:\n%s", check.path, doc)
			}
		}
	}
}

// structFieldDoc returns the doc comment of one struct field, read through the
// parser so that only the field's own comment counts.
func structFieldDoc(t *testing.T, path, structName, fieldName string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	doc, found := "", false
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != structName {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == fieldName {
					doc, found = field.Doc.Text(), true
				}
			}
		}
		return false
	})
	if !found {
		t.Fatalf("%s has no %s.%s", path, structName, fieldName)
	}
	return doc
}

func undocumented(t *testing.T, fset *token.FileSet, what string, pos token.Pos) {
	t.Helper()
	t.Errorf("%s: exported %s has no doc comment", fset.Position(pos), what)
}

func hasDoc(groups ...*ast.CommentGroup) bool {
	for _, group := range groups {
		if group != nil && strings.TrimSpace(group.Text()) != "" {
			return true
		}
	}
	return false
}

// flattenDoc collapses a comment or document to one line, so a phrase that
// wraps across source lines still matches.
func flattenDoc(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// readDocFile reads one repository document relative to the pdfgen directory.
func readDocFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// TestAssetConfinementDocumentation guards the documented asset rule: the
// public field docs state the root precedence and the opt-in, and the user
// documentation repeats the same enforced rule. The behaviour these sentences
// describe is covered by the engine's asset-root tests and by the source tests.
func TestAssetConfinementDocumentation(t *testing.T) {
	for _, check := range []struct {
		field string
		want  []string
	}{
		{"BaseDir", []string{"directory of File", "working directory", "confined"}},
		{"FontDir", []string{"FontDir, then BaseDir", "rejected"}},
		{"AllowAbsoluteAssets", []string{"read as written", "FallbackFont", "Source"}},
	} {
		doc := flattenDoc(structFieldDoc(t, "pdfgen.go", "Options", check.field))
		for _, phrase := range check.want {
			if !strings.Contains(doc, phrase) {
				t.Errorf("Options.%s does not say %q:\n%s", check.field, phrase, doc)
			}
		}
	}

	checks := []struct {
		path string
		want []string
	}{
		{filepath.Join("..", "README.md"), []string{"directory of `Options.File`", "AllowAbsoluteAssets", "confined"}},
		{filepath.Join("..", "docs", "fonts.md"), []string{"directory of `Options.File`", "AllowAbsoluteAssets", "confines the read"}},
	}
	for _, check := range checks {
		text := flattenDoc(readDocFile(t, check.path))
		for _, phrase := range check.want {
			if !strings.Contains(text, phrase) {
				t.Errorf("%s does not say %q", check.path, phrase)
			}
		}
	}
}
