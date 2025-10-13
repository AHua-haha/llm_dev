package codebase

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func travelTs() {

	// codeBytes, err := os.ReadFile("example.go")
	// if err != nil {
	// 	panic(err)
	// }

	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))

	// tree := parser.Parse(codeBytes, nil)
}
