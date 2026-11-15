package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// core T1222: nothing the cli prints or sends names a record of Smeldr's own
// instance. Help and usage text, printed lines, refusals and the reasons the
// cli sends to the server are all string literals in this package, so the
// guard reads every one in the non-test files. A user's instance has its own
// records: there an address such as D142 is another record or none. Examples
// use a one-digit number (D1, Task:T4), which is a shape and not a record.

// textAddress matches the addresses a text used to cite: D12, T884, A398,
// G12, S3893, P401.
var textAddress = regexp.MustCompile(`\b[DTAGSP][0-9]{2,4}\b`)

func TestTextsNameNoRecordAddresses(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			checked++
			if a := textAddress.FindString(s); a != "" {
				t.Errorf("%s: a text names %s: %q", fset.Position(lit.Pos()), a, s)
			}
			return true
		})
	}
	if checked < 500 {
		t.Fatalf("checked only %d string literals; the guard is not reading the files", checked)
	}
}
