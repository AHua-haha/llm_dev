package impl

import (
	"fmt"
	"llm_dev/codebase/common"
	"os"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type symbol struct {
	pkg_name   string
	identifier string
}

type fileSymExtractOps struct {
	path    string
	data    []byte
	symbols []symbol
}

func (fileops *fileSymExtractOps) nodeOps(root *tree_sitter.Node) bool {
	Kind := root.Kind()
	switch Kind {
	case "source_file":
		return true
	case "package_clause":
		package_identifier := root.Child(1)
		start, end := package_identifier.ByteRange()
		fmt.Printf("%s\n", string(fileops.data[start:end]))
		return false
	case "type_declaration":
		start, end := root.ByteRange()
		fmt.Printf("%s\n", string(fileops.data[start:end]))
		return false
	case "method_declaration", "function_declaration":
		body := root.ChildByFieldName("body")
		start := root.StartByte()
		end := body.StartByte()
		fmt.Printf("%s\n", string(fileops.data[start:end]))
		return false
	default:
		return false
	}
}

func (fileops *fileSymExtractOps) extractSymbol() {
	data, err := os.ReadFile(fileops.path)
	fileops.data = data
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}

	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
	tree := parser.Parse(data, nil)
	common.Walk(tree.RootNode(), fileops.nodeOps)
}
