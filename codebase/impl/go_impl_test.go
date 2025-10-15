package impl

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	ignore "github.com/sabhiram/go-gitignore"
	"golang.org/x/tools/go/packages"
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

func Test_parseProject(t *testing.T) {
	t.Run("test go parse whole project", func(t *testing.T) {
		cfg := &packages.Config{
			Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
			Fset: token.NewFileSet(),
			Dir:  "/home/ahua/workspace/llm/llm_dev", // Change this
		}

		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			panic(err)
		}

		for _, pkg := range pkgs {
			fmt.Printf("📦 Package: %s\n", pkg.PkgPath)

			for i, file := range pkg.Syntax {
				fileName := pkg.GoFiles[i]
				fmt.Printf("\n🔍 File: %s\n", fileName)

				// Walk the AST of the file and pull info from pkg.TypesInfo
				ast.Inspect(file, func(n ast.Node) bool {
					ident, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					fmt.Printf("ident: %v\n", ident)

					// Check if it's a used identifier (like a type reference)
					if obj, ok := pkg.TypesInfo.Uses[ident]; ok {
						fmt.Printf("obj.String(): %v\n", obj.String())
						pos := obj.Pos()
						p := cfg.Fset.Position(pos)
						if p.Filename != fileName {
							fmt.Printf("%s , %s\n", p.Filename, ident.Name)
						}
					}
					return true
				})
			}
		}
	})
}

func Test_WalkDir(t *testing.T) {
	t.Run("test walk dir with gitignore", func(t *testing.T) {
		root := "/home/ahua/workspace/llm/llm_dev"
		ig, err := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
		if err != nil {
			return
		}
		walkFunc := func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Get path relative to root
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}

			// Skip root
			if relPath == "." {
				return nil
			}

			// Check if the file/dir is ignored
			if ig.MatchesPath(relPath) {
				if d.IsDir() {
					// Skip entire directory
					return filepath.SkipDir
				}
				return nil
			}

			// Process the file/dir
			fmt.Println("Included:", relPath)

			return nil
		}
		err = filepath.WalkDir(root, walkFunc)
		if err != nil {
			return
		}
	})
}
