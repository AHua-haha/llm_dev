package common

import tree_sitter "github.com/tree-sitter/go-tree-sitter"

type NodeType int
type NodeOperation func(root *tree_sitter.Node) bool

type Node interface {
	nodeType() NodeType
	child() []Node
}

type file struct {
	filepath string
}

type symbolID interface {
}
