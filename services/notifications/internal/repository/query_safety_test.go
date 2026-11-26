package repository

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"
)

// ddlFuncs su jedine funkcije u kojima se CQL sklapa od teksta: ime
// keyspace-a ne može biti parametar DDL upita, pa se proverava regexom
// (config.Load i Migrate) pre nego što uđe u tekst.
var ddlFuncs = map[string]bool{"SchemaStatements": true, "Migrate": true}

// TestQueriesAreNotBuiltFromStrings je statička provera repozitorijuma (US-66):
// svaki CQL upit je konstanta sa ? parametrima, a korisnički ulaz (userID,
// kursor, limit) ide samo kao vezana vrednost. Van DDL funkcija nema
// fmt.Sprintf ni spajanja stringova.
func TestQueriesAreNotBuiltFromStrings(t *testing.T) {
	consts := map[string]bool{}
	fset := token.NewFileSet()
	var files []*ast.File
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
				for _, s := range g.Specs {
					for _, n := range s.(*ast.ValueSpec).Names {
						consts[n.Name] = true
					}
				}
			}
		}
	}

	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || ddlFuncs[fn.Name.Name] {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					sel, ok := x.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" && strings.HasPrefix(sel.Sel.Name, "Sprint") {
						t.Errorf("%s: fmt.%s u repozitorijumu", fset.Position(x.Pos()), sel.Sel.Name)
					}
					if sel.Sel.Name == "Query" && len(x.Args) > 0 {
						switch a := x.Args[0].(type) {
						case *ast.BasicLit:
						case *ast.Ident:
							if !consts[a.Name] {
								t.Errorf("%s: CQL upit %q nije konstanta", fset.Position(x.Pos()), a.Name)
							}
						default:
							t.Errorf("%s: CQL upit nije konstanta", fset.Position(x.Pos()))
						}
					}
				case *ast.BinaryExpr:
					if x.Op == token.ADD {
						if lit, ok := x.X.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							t.Errorf("%s: spajanje stringova u repozitorijumu", fset.Position(x.Pos()))
						}
					}
				}
				return true
			})
		}
	}
}

func TestMigrateRejectsUnsafeKeyspace(t *testing.T) {
	for _, ks := range []string{"", "Notifications", "a; DROP KEYSPACE system", "a.b", "a b", strings.Repeat("a", 49)} {
		if err := Migrate(context.Background(), nil, ks, 1, time.Second); err == nil {
			t.Errorf("keyspace %q je prihvaćen", ks)
		}
	}
}
