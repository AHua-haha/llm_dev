package common

import (
	"io/fs"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type AstNodeOps func(root *tree_sitter.Node) bool

type FileOps func(path string, d fs.DirEntry) (Node, bool)

type NodeOps func(node Node) bool

type Node interface {
	child() []Node
	addChild(node Node)
}

type file struct {
	filepath string
}

type symbolID interface {
}
