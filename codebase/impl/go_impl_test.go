package impl

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func Test_fileSymExtractOps_extractSymbol(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		fileops fileSymExtractOps
	}{
		// TODO: Add test cases.
		{
			name: "test extract operation",
			fileops: fileSymExtractOps{
				path: "go_impl.go",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			tt.fileops.extractSymbol()
		})
	}
}

func Test_goTypeChecker(t *testing.T) {
	t.Run("test go ast type", func(t *testing.T) {
		src := `
package main

import "fmt"

func add(a int, b int) int {
	return a + b
}
type test struct {
	a int
	b int
	c string
}

func main() {
	var tt test
	tt.a = 22
	var x int = 10
	y := 20
	z := "hello"
	fmt.Println(add(x, y), z)
}
`

		fset := token.NewFileSet()

		// Parse the source file
		file, err := parser.ParseFile(fset, "example.go", src, parser.AllErrors)
		if err != nil {
			panic(err)
		}

		// Prepare type info storage
		info := &types.Info{
			Defs:  make(map[*ast.Ident]types.Object),
			Uses:  make(map[*ast.Ident]types.Object),
			Types: make(map[ast.Expr]types.TypeAndValue),
		}

		// Type-check the package
		conf := types.Config{
			Importer: importer.Default(),
			Error: func(err error) {
				fmt.Printf("type err: %v\n", err)
			},
		}
		_, err = conf.Check("mypkg", fset, []*ast.File{file}, info)
		if err != nil {
			panic(err)
		}
		for _, obj := range info.Uses {
			pos := obj.Pos()
			fmt.Printf("fset.Position(pos): %v\n", fset.Position(pos))
			switch obj := obj.(type) {
			case *types.TypeName:
				fmt.Printf("obj.Name(): %v\n", obj.Name())
			case *types.Func:
				fmt.Printf("obj.Name(): %v\n", obj.Name())
				pkg := obj.Pkg()
				if pkg == nil {
					continue
				}
				fmt.Printf("pkg.Path(): %v\n", pkg.Path())
			}
		}
	})
}
