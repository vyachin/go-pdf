package pdfgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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
