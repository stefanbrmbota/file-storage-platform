package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestQueriesAreNotBuiltFromStrings je statička provera repozitorijuma (US-66):
// upiti ka MongoDB-u se grade samo od bson.M / bson.D sa ključevima koji su
// literali, a vrednosti (korisnički ulaz) idu samo kao vrednosti. Nema
// fmt.Sprintf ni spajanja stringova, pa ulaz kao {"$ne": ""} ili "x' || '1"
// ne može da postane deo upita (operator ili ime polja).
func TestQueriesAreNotBuiltFromStrings(t *testing.T) {
	for _, problem := range scanRepository(t, ".") {
		t.Error(problem)
	}
}

func scanRepository(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var problems []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, dir+"/"+name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" && strings.HasPrefix(sel.Sel.Name, "Sprint") {
						problems = append(problems, fset.Position(x.Pos()).String()+": fmt."+sel.Sel.Name+" u repozitorijumu")
					}
				}
			case *ast.BinaryExpr:
				if x.Op == token.ADD && (isString(x.X) || isString(x.Y)) {
					problems = append(problems, fset.Position(x.Pos()).String()+": spajanje stringova u repozitorijumu")
				}
			case *ast.KeyValueExpr:
				// bson.E{Key: ..., Value: ...}: ključ (ime polja ili operator) mora biti literal.
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == "Key" {
					if _, lit := x.Value.(*ast.BasicLit); !lit {
						problems = append(problems, fset.Position(x.Pos()).String()+": bson Key nije literal")
					}
				}
			case *ast.CompositeLit:
				// bson.M{...}: svaki ključ mora biti literal.
				if sel, ok := x.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "M" {
					for _, el := range x.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							if _, lit := kv.Key.(*ast.BasicLit); !lit {
								problems = append(problems, fset.Position(kv.Pos()).String()+": bson.M ključ nije literal")
							}
						}
					}
				}
			}
			return true
		})
	}
	return problems
}

func isString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}
